package unpackerr

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
)

const maxActionBody = 4096

var (
	errQueueNotFound  = errors.New("queue item not found")
	errQueueNotFailed = errors.New("queue item is not extractfailed")
	errMissingID      = errors.New("id is required")
)

type idRequest struct {
	ID string `json:"id"`
}

func readIDRequest(response http.ResponseWriter, request *http.Request) (string, bool) {
	var body idRequest
	if err := json.NewDecoder(io.LimitReader(request.Body, maxActionBody)).Decode(&body); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return "", false
	}

	body.ID = strings.TrimSpace(body.ID)
	if body.ID == "" {
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

	if err := u.retryQueueID(itemID); err != nil {
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

	if err := u.forgetQueueID(itemID); err != nil {
		writeQueueActionError(response, err)
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"status": "ok", "id": itemID})
}

func (u *Unpackerr) historyClearHandler(response http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	u.clearHistory()
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (u *Unpackerr) historyDeleteHandler(response http.ResponseWriter, request *http.Request, _ httprouter.Params) {
	itemID, ok := readIDRequest(response, request)
	if !ok {
		return
	}

	if !u.deleteHistoryID(itemID) {
		writeJSON(response, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	writeJSON(response, http.StatusOK, map[string]string{"status": "ok", "id": itemID})
}

func writeQueueActionError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errQueueNotFound):
		writeJSON(response, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, errQueueNotFailed):
		writeJSON(response, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func (u *Unpackerr) retryQueueID(itemID string) error {
	u.lockHistory()
	defer u.unlockHistory()

	item, ok := u.Map[itemID]
	if !ok || item == nil {
		return errQueueNotFound
	}

	if item.Status != EXTRACTFAILED {
		return errQueueNotFailed
	}

	u.Retries++
	item.Retries++
	item.NoRetry = false
	item.Status = WAITING
	item.Updated = time.Now()

	return nil
}

func (u *Unpackerr) forgetQueueID(itemID string) error {
	u.lockHistory()
	defer u.unlockHistory()

	if _, ok := u.Map[itemID]; !ok {
		return errQueueNotFound
	}

	delete(u.Map, itemID)

	if u.folders != nil {
		delete(u.folders.Folders, itemID)
	}

	return nil
}
