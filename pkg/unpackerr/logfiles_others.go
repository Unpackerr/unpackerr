//go:build !windows

package unpackerr

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func getFileOwner(fileInfo os.FileInfo) string {
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	gid := strconv.FormatUint(uint64(stat.Gid), 10)
	usr := ""
	grp := ""

	if userName, err := user.LookupId(uid); err == nil {
		usr = userName.Username
	}

	if groupName, err := user.LookupGroupId(gid); err == nil {
		grp = groupName.Name
	}

	if usr != "" {
		return usr + ":" + grp + " (" + uid + ":" + gid + ")"
	}

	return uid + ":" + gid
}
