package folders

import (
	"os"
	"path/filepath"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
)

func expandHomedir(filePath string) string {
	expanded, err := homedir.Expand(filePath)
	if err != nil {
		return filePath
	}

	return expanded
}

// Check stats all configured folders and returns only "good" ones.
func Check(list []*FolderConfig, log Logs) ([]*FolderConfig, []string) {
	var (
		err         error
		goodFolders = list[:0]
		goodFlist   = []string{}
	)

	for _, folder := range list {
		folder.Path, err = filepath.Abs(expandHomedir(folder.Path))
		if err != nil {
			log.Errorf("Folder '%s' (bad path): %v", folder.Path, err)
			continue
		}

		if folder.ExtractPath != "" {
			folder.ExtractPath, err = filepath.Abs(expandHomedir(folder.ExtractPath))
			if err != nil {
				log.Errorf("Folder '%s' (bad extract path): %v", folder.ExtractPath, err)
				continue
			}
		}

		folder.ExcludePaths = NormalizeExcludePaths(folder.Path, folder.ExcludePaths)

		if stat, err := os.Stat(folder.Path); err != nil {
			log.Errorf("Folder '%s' (cannot watch): %v", folder.Path, err)
			continue
		} else if !stat.IsDir() {
			log.Errorf("Folder '%s' (cannot watch): not a folder", folder.Path)
			continue
		}

		goodFolders = append(goodFolders, folder)
		goodFlist = append(goodFlist, folder.Path)
	}

	return goodFolders, goodFlist
}

// NormalizeExcludePaths expands and cleans exclude paths relative to a watch folder.
func NormalizeExcludePaths(basePath string, excludes []string) []string {
	cleaned := make([]string, 0, len(excludes))

	for _, exclude := range excludes {
		exclude = strings.TrimSpace(exclude)
		if exclude == "" {
			continue
		}

		exclude = expandHomedir(exclude)
		if !filepath.IsAbs(exclude) {
			exclude = filepath.Join(basePath, exclude)
		}

		if abs, err := filepath.Abs(exclude); err == nil {
			cleaned = append(cleaned, filepath.Clean(abs))
		}
	}

	return cleaned
}

// IsExcludedPath returns true if path is the exclude or a child of it.
func (c *FolderConfig) IsExcludedPath(path string) bool {
	if len(c.ExcludePaths) == 0 || path == "" {
		return false
	}

	path = filepath.Clean(path)

	for _, exclude := range c.ExcludePaths {
		exclude = filepath.Clean(exclude)
		if path == exclude || strings.HasPrefix(path, exclude+string(os.PathSeparator)) {
			return true
		}
	}

	return false
}
