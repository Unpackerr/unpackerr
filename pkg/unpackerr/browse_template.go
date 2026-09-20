package unpackerr

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Unpackerr/unpackerr/pkg/hooks"
)

var (
	errMissingTemplate  = errors.New("template is required")
	errUnknownTemplate  = errors.New("unknown template")
	errTemplateExists   = errors.New("file already exists")
	errTemplateFileName = errors.New("path must use the locked template file name")
)

type browseTemplateRequest struct {
	Path     string `json:"path"`
	Template string `json:"template"`
}

type browseTemplateResponse struct {
	Path string `json:"path"`
}

func (u *Unpackerr) browseTemplateHandler(response http.ResponseWriter, request *http.Request) {
	var body browseTemplateRequest
	if !readJSON(response, request, maxBrowseBody, &body) {
		return
	}

	if strings.TrimSpace(body.Path) == "" {
		writeTemplateError(response, errMissingPath)
		return
	}

	if strings.TrimSpace(body.Template) == "" {
		writeTemplateError(response, errMissingTemplate)
		return
	}

	u.Printf("[user requested] Writing webhook template %s: %s", body.Template, body.Path)

	path, err := writeWebhookTemplate(body.Path, body.Template)
	if err != nil {
		writeTemplateError(response, err)
		return
	}

	writeJSON(response, http.StatusCreated, browseTemplateResponse{Path: path})
}

func writeTemplateError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errTemplateExists):
		writeJSON(response, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, errUnknownTemplate),
		errors.Is(err, errMissingTemplate),
		errors.Is(err, errMissingPath),
		errors.Is(err, errTemplateFileName):
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writeWebhookTemplate(path, name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "default" {
		return "", errUnknownTemplate
	}

	body, ok := hooks.BuiltinWebhookTemplate(name)
	if !ok {
		return "", errUnknownTemplate
	}

	path = expandBrowsePath(path)

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("unable to write template: %w", err)
	}

	path = abs
	want := hooks.WebhookTemplateFileName(name)

	if filepath.Base(path) != want {
		return "", fmt.Errorf("%w: %s", errTemplateFileName, want)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL

	file, err := os.OpenFile(path, flags, defaultFileMode)
	if err != nil {
		if os.IsExist(err) {
			return "", errTemplateExists
		}

		return "", fmt.Errorf("unable to write template: %w", err)
	}

	_, err = file.WriteString(body)
	closeErr := file.Close()

	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("unable to write template: %w", err)
	}

	if closeErr != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("unable to write template: %w", closeErr)
	}

	return path, nil
}
