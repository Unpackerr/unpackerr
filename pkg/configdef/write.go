package configdef

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var errEmptyConfigPath = errors.New("config path is empty")

const (
	configFileMode = 0o600
	backupSuffix   = ".bak"
)

// AtomicWrite writes data to path by temp file + rename, keeping path.bak.
func AtomicWrite(path string, data []byte) error {
	return atomicWrite(path, data, true)
}

// AtomicReplace writes data to path by temp file + rename without creating path.bak.
func AtomicReplace(path string, data []byte) error {
	return atomicWrite(path, data, false)
}

func atomicWrite(path string, data []byte, backup bool) error {
	if path == "" {
		return errEmptyConfigPath
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("making config dir: %w", err)
	}

	mode := os.FileMode(configFileMode)
	if stat, err := os.Stat(path); err == nil {
		mode = stat.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, ".unpackerr-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp config: %w", err)
	}

	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	_ = tmp.Chmod(mode)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp config: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing temp config: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp config: %w", err)
	}

	if err := replaceFile(path, tmpName, mode, backup); err != nil {
		return err
	}

	tmpName = "" // replaced; defer remove is a no-op

	return nil
}

func replaceFile(path, tmpName string, mode os.FileMode, backup bool) error {
	bak := path + backupSuffix

	if backup {
		if _, err := os.Stat(path); err == nil {
			if err := copyFile(path, bak, mode); err != nil {
				return fmt.Errorf("backing up config: %w", err)
			}
		}
	}

	if err := os.Rename(tmpName, path); err == nil {
		return nil
	}

	_ = os.Remove(path)

	if err := os.Rename(tmpName, path); err != nil {
		if backup {
			_ = copyFile(bak, path, mode)
		}

		return fmt.Errorf("replacing config: %w", err)
	}

	return nil
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open backup dest: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("copy backup: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		_ = dstFile.Close()
		return fmt.Errorf("sync backup: %w", err)
	}

	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("close backup: %w", err)
	}

	return nil
}
