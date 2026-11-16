//go:build linux

package main

import (
	"golang.org/x/sys/unix"
)

// hardenProcessDumpability restricts /proc/<pid>/environ, /proc/<pid>/mem,
// and /proc/<pid>/maps permissions to 0400 root:root on Linux.
// This prevents same-UID child processes (such as malicious npm/bun dependencies)
// from reading ambient secrets from the parent supervisor's process environment block.
func hardenProcessDumpability() {
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
}

