//go:build linux
// +build linux

package utils

import (
	"os"
	"syscall"
)

// FICLONE is the ioctl command for copy-on-write cloning on Linux.
const FICLONE = 0x40049409

// Reflink attempts to create a copy-on-write clone of src at dst.
// It returns an error if cloning fails.
func Reflink(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// ioctl(dst_fd, FICLONE, src_fd)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, dstFile.Fd(), FICLONE, srcFile.Fd())
	if errno != 0 {
		return errno
	}

	return nil
}
