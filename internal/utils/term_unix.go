//go:build !windows

package utils

import (
	"fmt"
)

func initTerminal() {
	// Virtual terminal processing is not needed on Unix-like systems.
}

func ClearTerminal() {
	fmt.Print("\033[H\033[2J\033[3J")
}
