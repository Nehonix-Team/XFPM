//go:build !linux

package core

import "os"

func getSyscallInode(fi os.FileInfo) string {
	return ""
}
