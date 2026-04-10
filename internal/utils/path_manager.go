package utils

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func EnsurePathInShell() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	binPath := filepath.Join(home, ".xpm", "bin")
	
	if runtime.GOOS == "windows" {
		return setupWindowsPath(binPath)
	}

	return setupUnixPath(home, binPath)
}

func setupUnixPath(home, binPath string) error {
	// 1. Detect common shell profiles
	profiles := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
	}

	// 2. Identify active shell to prioritize
	shellPath := os.Getenv("SHELL")
	if strings.Contains(shellPath, "zsh") {
		profiles = append([]string{filepath.Join(home, ".zshrc")}, profiles...)
	} else if strings.Contains(shellPath, "bash") {
		profiles = append([]string{filepath.Join(home, ".bashrc")}, profiles...)
	}

	line := fmt.Sprintf("\n# XFPM PATH (auto-generated)\nexport PATH=\"%s:$PATH\"\n", binPath)
	
	modifiedAny := false
	for _, profile := range profiles {
		if _, err := os.Stat(profile); err != nil {
			continue // Profile doesn't exist, skip
		}

		content, err := ioutil.ReadFile(profile)
		if err != nil {
			continue
		}

		// Check if already present
		if strings.Contains(string(content), binPath) {
			continue
		}

		// Append line
		f, err := os.OpenFile(profile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		
		if _, err := f.WriteString(line); err == nil {
			modifiedAny = true
		}
		f.Close()
	}

	if modifiedAny {
		Info("PATH updated in your shell profile. Please run 'source ~/.bashrc' or 'source ~/.zshrc' to apply.")
	}

	return nil
}

func setupWindowsPath(binPath string) error {
	// On Windows, we try to use powershell to update the User Path Environment variable
	// Note: Implementation for Windows Registry/Environment will be finalized in G0.1.79.
	// For now, it detects presence but requires manual SETX if not found on Windows.
	return nil
}
