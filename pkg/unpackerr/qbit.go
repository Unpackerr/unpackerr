package unpackerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	// qbitMaxBody caps a qBittorrent reply. A file list of a few thousand entries is well under it.
	qbitMaxBody = 32 << 20
	// qbitRetry is how long to leave qBittorrent alone after it could not be reached.
	qbitRetry = 5 * time.Minute
)

var (
	errQbitURL     = errors.New("qbit_url must begin with http:// or https://")
	errQbitLogin   = errors.New("qbittorrent rejected the login")
	errQbitPaused  = errors.New("qbittorrent unreachable recently, not retrying yet")
	errQbitUnknown = errors.New("torrent not found in qbittorrent")
	errQbitStatus  = errors.New("unexpected qbittorrent response")
)

// qbitFile is one entry from qBittorrent's torrents/files endpoint (Web API 2.0 and later).
// Name is relative to the torrent's save path and starts with the torrent's root folder,
// if it has one. Priority 0 means the file is not selected for download.
type qbitFile struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Priority int    `json:"priority"`
}

// qbitClient is the small part of the qBittorrent Web API that Unpackerr needs:
// cookie login and a torrent's file list. It works with Web API 2.x (qBittorrent 4.1+).
// Only the main loop uses it.
type qbitClient struct {
	url    string // base URL with trailing slash, including any URL base path.
	user   string
	pass   string
	client *http.Client
	// rejected is set when qBittorrent refused the credentials. Retrying the same
	// credentials only counts toward its failed-login ban (which also locks out
	// every other client from this address), so stop until the config changes.
	rejected bool
	// retryAt holds off requests after a network or server error.
	retryAt time.Time
}

// newQbitClient has no client timeout: every call carries a context deadline.
func newQbitClient(base, user, pass string) *qbitClient {
	jar, _ := cookiejar.New(nil) // never errors without options.

	return &qbitClient{
		url:    base,
		user:   user,
		pass:   pass,
		client: &http.Client{Jar: jar},
	}
}

// files returns the file list for a torrent hash. It logs in when the session is
// missing or expired, and returns errQbitUnknown when qBittorrent has no such torrent.
func (q *qbitClient) files(ctx context.Context, hash string) ([]qbitFile, error) {
	switch {
	case q.rejected:
		return nil, errQbitLogin
	case time.Now().Before(q.retryAt):
		return nil, errQbitPaused
	}

	files, err := q.fetchFiles(ctx, hash)

	switch {
	case errors.Is(err, errQbitLogin):
		q.rejected = true
	case err != nil && !errors.Is(err, errQbitUnknown):
		q.retryAt = time.Now().Add(qbitRetry)
	}

	return files, err
}

func (q *qbitClient) fetchFiles(ctx context.Context, hash string) ([]qbitFile, error) {
	query := url.Values{"hash": {hash}}
	uri := q.url + "api/v2/torrents/files?" + query.Encode()

	files, status, err := q.getFiles(ctx, uri)
	if err == nil && (status == http.StatusForbidden || status == http.StatusUnauthorized) {
		if err = q.login(ctx); err != nil {
			return nil, err
		}

		files, status, err = q.getFiles(ctx, uri)
	}

	switch {
	case err != nil:
		return nil, err
	case status == http.StatusNotFound, status == http.StatusConflict:
		return nil, errQbitUnknown
	case status != http.StatusOK:
		return nil, fmt.Errorf("%w: torrents/files: %d", errQbitStatus, status)
	case len(files) == 0:
		return nil, errQbitUnknown // no metadata yet (magnet), nothing to compare.
	default:
		return files, nil
	}
}

// getFiles runs one torrents/files request. A non-200 status is returned without an error.
func (q *qbitClient) getFiles(ctx context.Context, uri string) ([]qbitFile, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("qbittorrent request: %w", err)
	}

	req.Header.Set("Referer", q.url)

	resp, err := q.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("qbittorrent request: %w", err)
	}

	defer drain(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}

	var files []qbitFile
	if err := json.NewDecoder(io.LimitReader(resp.Body, qbitMaxBody)).Decode(&files); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decoding qbittorrent file list: %w", err)
	}

	return files, resp.StatusCode, nil
}

// login posts the credentials. Older qBittorrent replies 200 "Ok." or 200 "Fails.";
// 5.2 and later reply 204 on success. The Referer must match the Web UI address or
// CSRF protection rejects the request. Errors never include the password.
func (q *qbitClient) login(ctx context.Context) error {
	form := url.Values{"username": {q.user}, "password": {q.pass}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.url+"api/v2/auth/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("qbittorrent login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.url)

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent login: %w", err)
	}

	defer drain(resp)

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64)) //nolint:mnd // "Ok." or "Fails.".

	switch {
	case resp.StatusCode == http.StatusNoContent:
		return nil
	case resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == "Ok.":
		return nil
	case resp.StatusCode == http.StatusOK, // "Fails."
		resp.StatusCode == http.StatusForbidden, // banned after too many failures.
		resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", errQbitLogin, resp.Status)
	default:
		return fmt.Errorf("%w: auth/login: %s", errQbitStatus, resp.Status)
	}
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, qbitMaxBody))
	_ = resp.Body.Close()
}

// normalizeQbitURL checks and tidies the qbit_url setting. Empty disables the check.
func normalizeQbitURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return "", errQbitURL
	}

	return strings.TrimSuffix(raw, "/") + "/", nil
}
