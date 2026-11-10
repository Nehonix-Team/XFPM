//go:build linux

package core

import (
	"fmt"
	"os"
	"syscall"
)

func getSyscallInode(fi os.FileInfo) string {
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
	}
	return ""
}

