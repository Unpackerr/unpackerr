package gorar

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"github.com/nwaples/rardecode"
	"archive/zip"
	"runtime"
	"strings"
)

// Extract rar/zip files.
//See README for example's.

func SayHello(word string) string {
	fmt.Println("hello world!")
	return "hello"
}

func RarExtractor(path string, destination string) error {

	rr, err := rardecode.OpenReader(path, "")

	if err != nil {
		return fmt.Errorf("read: failed to create reader: %v", err)
	}

	//sum := 1
	for {
		//sum += sum
		header, err := rr.Next()
		if err == io.EOF {
			break
		}

		if header.IsDir {
			err = mkdir(filepath.Join(destination, header.Name))
			if err != nil {
				return err
			}
			continue
		}
		err = mkdir(filepath.Dir(filepath.Join(destination, header.Name)))
		if err != nil {
			return err
		}

		err = writeNewFile(filepath.Join(destination, header.Name), rr, header.Mode())
		if err != nil {
			return err
		}

	}

	return nil
}

func ZipExtractor(source string, destination string) error {

	r, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer r.Close()

	return unzipAll(&r.Reader, destination)
}

func unzipAll(r *zip.Reader, destination string) error {
	for _, zf := range r.File {
		if err := unzipFile(zf, destination); err != nil {
			return err
		}
	}

	return nil
}

func unzipFile(zf *zip.File, destination string) error {
	if strings.HasSuffix(zf.Name, "/") {
		return mkdir(filepath.Join(destination, zf.Name))
	}

	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("%s: open compressed file: %v", zf.Name, err)
	}
	defer rc.Close()

	return writeNewFile(filepath.Join(destination, zf.Name), rc, zf.FileInfo().Mode())
}

func mkdir(dirPath string) error {
	err := os.MkdirAll(dirPath, 0755)
	if err != nil {
		return fmt.Errorf("%s: making directory: %v", dirPath, err)
	}
	return nil
}

func writeNewFile(fpath string, in io.Reader, fm os.FileMode) error {
	err := os.MkdirAll(filepath.Dir(fpath), 0755)
	if err != nil {
		return fmt.Errorf("%s: making directory for file: %v", fpath, err)
	}

	out, err := os.Create(fpath)
	if err != nil {
		return fmt.Errorf("%s: creating new file: %v", fpath, err)
	}
	defer out.Close()

	err = out.Chmod(fm)
	if err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("%s: changing file mode: %v", fpath, err)
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("%s: writing file: %v", fpath, err)
	}
	return nil
}
