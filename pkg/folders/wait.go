package folders

import (
	"os"
	"strings"
)

// NormalizeWaitExtensions trims empty values and forces a leading period.
func NormalizeWaitExtensions(exts []string) []string {
	if len(exts) == 0 {
		return nil
	}

	out := make([]string, 0, len(exts))
	seen := make(map[string]struct{}, len(exts))

	for _, ext := range exts {
		if ext = strings.TrimSpace(ext); ext == "" || ext == "." {
			continue
		}

		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}

		key := strings.ToLower(ext)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		out = append(out, ext)
	}

	return out
}

// WaitFileInTop returns the first non-directory name in path that ends with a
// wait extension. It does not walk subfolders.
func WaitFileInTop(path string, exts []string) string {
	exts = NormalizeWaitExtensions(exts)
	if path == "" || len(exts) == 0 {
		return ""
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if name := entry.Name(); hasWaitSuffix(name, exts) {
			return name
		}
	}

	return ""
}

func hasWaitSuffix(name string, exts []string) bool {
	name = strings.ToLower(name)

	for _, ext := range exts {
		if ext != "" && strings.HasSuffix(name, strings.ToLower(ext)) {
			return true
		}
	}

	return false
}
