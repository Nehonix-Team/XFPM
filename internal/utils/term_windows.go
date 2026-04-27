//go:build windows

package utils

import (
	"fmt"
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

func ClearTerminal() {
	fmt.Print("\033[H\033[2J\033[3J")
}
