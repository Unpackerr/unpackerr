//go:build windows

package unpackerr

import "testing"

func TestFolderPathContainsWindowsRoot(t *testing.T) {
	t.Parallel()

	if !folderPathContains(`C:\`, `C:\downloads\movie`) {
		t.Fatal(`C:\ should contain C:\downloads\movie`)
	}

	if !folderPathContains(`\`, `\downloads\movie`) {
		t.Fatal(`\ should contain \downloads\movie`)
	}
}
