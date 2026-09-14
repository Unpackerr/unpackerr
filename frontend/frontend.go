package frontend

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const unbuiltIndex = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Unpackerr</title></head>
<body>
<p>Unpackerr UI is not included in this build.</p>
</body>
</html>
`

//nolint:gochecknoglobals
var (
	handler http.Handler
	root    fs.FS
	//go:embed all:dist
	embedded embed.FS
)

//nolint:gochecknoinits
func init() {
	root, _ = fs.Sub(embedded, "dist")
	handler = http.FileServerFS(root)
}

type responseWriter struct {
	http.ResponseWriter
	asset     bool
	sendIndex bool
}

// IndexHandler returns a built asset if it exists, otherwise index.html (SPA).
func IndexHandler(resp http.ResponseWriter, req *http.Request) {
	name := strings.TrimPrefix(req.URL.Path, "/")
	if name == "" || name == "index.html" {
		req.URL.Path = "/"
		name = "index.html"
	} else {
		req.URL.Path = "/" + name
	}

	if _, err := fs.Stat(root, "index.html"); err != nil {
		if isAsset(name) {
			http.NotFound(resp, req)
			return
		}

		resp.Header().Set("Content-Type", "text/html; charset=utf-8")
		resp.Header().Set("Cache-Control", "no-cache")
		_, _ = resp.Write([]byte(unbuiltIndex))

		return
	}

	response := &responseWriter{
		ResponseWriter: resp,
		asset:          isAsset(name),
	}
	handler.ServeHTTP(response, req)

	if !response.sendIndex {
		return
	}

	resp.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFileFS(resp, req, root, "index.html")
}

func isAsset(filepath string) bool {
	return strings.HasPrefix(filepath, "assets/") || map[string]bool{
		".json":  true,
		".svg":   true,
		".png":   true,
		".jpg":   true,
		".ico":   true,
		".webp":  true,
		".map":   true,
		".css":   true,
		".js":    true,
		".woff":  true,
		".woff2": true,
	}[strings.ToLower(path.Ext(filepath))]
}

func (w *responseWriter) WriteHeader(status int) {
	w.ResponseWriter.Header().Set("Cache-Control", "no-cache")

	if status != http.StatusNotFound {
		if w.asset {
			w.ResponseWriter.Header().Set("Cache-Control", "max-age=31536000")
		}

		w.ResponseWriter.WriteHeader(status)

		return
	}

	if w.asset {
		w.ResponseWriter.WriteHeader(http.StatusNotFound)
		return
	}

	w.sendIndex = true
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.sendIndex {
		return w.ResponseWriter.Write(p) //nolint:wrapcheck
	}

	return len(p), nil
}
