//go:build windows
// +build windows

package utils

import (
	"os"

	"golang.org/x/sys/windows"
)

func initTerminal() {
	stdout := windows.Handle(os.Stdout.Fd())
	var originalMode uint32
	windows.GetConsoleMode(stdout, &originalMode)
	windows.SetConsoleMode(stdout, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	
	stderr := windows.Handle(os.Stderr.Fd())
	var originalModeErr uint32
	windows.GetConsoleMode(stderr, &originalModeErr)
	windows.SetConsoleMode(stderr, originalModeErr|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
}
