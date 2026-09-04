package unpackerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
)

const maxActionBody = 4096

type queueActionKind byte

const (
	queueRetry queueActionKind = iota + 1
	queueForget
)

type queueAction struct {
	kind   queueActionKind
	id     string
	result chan error
}

var (
	errQueueNotFound       = errors.New("queue item not found")
	errQueueNotFailed      = errors.New("queue item is not extractfailed")
	errQueueNotForgettable = errors.New("queue item is still in progress")
	errUnknownQueueAction  = errors.New("unknown queue action")
	errMissingID           = errors.New("id is required")
)

type idRequest struct {
	ID string `json:"id"`
}

func readIDRequest(response http.ResponseWriter, request *http.Request) (string, bool) {
	var body idRequest

	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, maxActionBody))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return "", false
	}

	err := decoder.Decode(&struct{}{})
	if err != nil && !errors.Is(err, io.EOF) {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return "", false
	}

	if err == nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": errExtraJSON.Error()})
		return "", false
	}

	if strings.TrimSpace(body.ID) == "" {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": errMissingID.Error()})
		return "", false
	}

	return body.ID, true
}

func (u *Unpackerr) queueRetryHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	itemID, ok := readIDRequest(response, request)
	if !ok {
		return
	}

	if err := u.dispatchQueueAction(request.Context(), queueRetry, itemID); err != nil {
		writeQueueActionError(response, err)
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"status": "ok", "id": itemID})
}

func (u *Unpackerr) queueForgetHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	itemID, ok := readIDRequest(response, request)
	if !ok {
		return
	}

	if err := u.dispatchQueueAction(request.Context(), queueForget, itemID); err != nil {
		writeQueueActionError(response, err)
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"status": "ok", "id": itemID})
}

func (u *Unpackerr) historyClearHandler(response http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	if err := u.clearHistory(); err != nil {
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (u *Unpackerr) historyDeleteHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	itemID, ok := readIDRequest(response, request)
	if !ok {
		return
	}

	if err := u.deleteHistoryID(itemID); err != nil {
		writeJSON(response, historyDeleteCode(err), map[string]string{"error": err.Error()})
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"status": "ok", "id": itemID})
}

func historyDeleteCode(err error) int {
	if errors.Is(err, errHistoryNotFound) {
		return http.StatusNotFound
	}

	return http.StatusInternalServerError
}

func writeQueueActionError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errQueueNotFound):
		writeJSON(response, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, errQueueNotFailed), errors.Is(err, errQueueNotForgettable):
		writeJSON(response, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeJSON(response, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
	default:
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func (u *Unpackerr) dispatchQueueAction(ctx context.Context, kind queueActionKind, id string) error {
	action := &queueAction{kind: kind, id: id, result: make(chan error, 1)}

	select {
	case u.queueActChan <- action:
	case <-ctx.Done():
		return fmt.Errorf("queue action: %w", ctx.Err())
	}

	select {
	case err := <-action.result:
		return err
	case <-ctx.Done():
		return fmt.Errorf("queue action: %w", ctx.Err())
	}
}

func (u *Unpackerr) runQueueActions(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case action := <-u.queueActChan:
			action.result <- u.applyQueueAction(action)
		}
	}
}

func (u *Unpackerr) applyQueueAction(action *queueAction) error {
	switch action.kind {
	case queueRetry:
		return u.retryQueueID(action.id)
	case queueForget:
		return u.forgetQueueID(action.id)
	default:
		return errUnknownQueueAction
	}
}

func (u *Unpackerr) retryQueueID(itemID string) error {
	now := time.Now()

	u.lockHistory()
	defer u.unlockHistory()

	item, ok := u.Map[itemID]
	if !ok || item == nil {
		return errQueueNotFound
	}

	if item.Status != EXTRACTFAILED {
		return errQueueNotFailed
	}

	if item.App == FolderString {
		return u.retryFolderLocked(itemID, item, now)
	}

	u.Retries++
	item.Retries++
	item.NoRetry = false
	item.Status = WAITING
	item.Updated = now

	return nil
}

func (u *Unpackerr) retryFolderLocked(itemID string, item *Extract, now time.Time) error {
	if u.folders == nil {
		return errQueueNotFound
	}

	folder, ok := u.folders.Folders[itemID]
	if !ok || folder == nil {
		return errQueueNotFound
	}

	folder.status = WAITING
	folder.noRetry = false
	folder.retries = 0
	folder.updated = now
	item.NoRetry = false
	item.Status = WAITING
	item.Updated = now

	return nil
}

func (u *Unpackerr) forgetQueueID(itemID string) error {
	u.lockHistory()
	defer u.unlockHistory()

	item, ok := u.Map[itemID]
	if !ok || item == nil {
		return errQueueNotFound
	}

	if !item.Status.isDurableHistory() {
		return errQueueNotForgettable
	}

	delete(u.Map, itemID)

	if item.App != FolderString {
		u.forgotten[itemID] = struct{}{}
	}

	if u.folders != nil {
		if _, exists := u.folders.Folders[itemID]; exists {
			u.folders.Remove(itemID)
			delete(u.folders.Folders, itemID)
		}
	}

	return nil
}

func (u *Unpackerr) isForgotten(id string) bool {
	_, ok := u.forgotten[id]
	return ok
}

func (u *Unpackerr) sweepForgotten() {
	u.lockHistory()
	defer u.unlockHistory()

	for itemID := range u.forgotten {
		if u.haveLidarrQitem(itemID) || u.haveRadarrQitem(itemID) ||
			u.haveReadarrQitem(itemID) || u.haveSonarrQitem(itemID) || u.haveWhisparrQitem(itemID) {
			continue
		}

		delete(u.forgotten, itemID)
	}
}
