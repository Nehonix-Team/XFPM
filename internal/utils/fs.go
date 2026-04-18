package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// CreateDirAllSecure creates a directory and all parent directories with 0755 permissions.
// On non-Windows systems, it explicitly applies Chmod to ensure permissions are set correctly.
func CreateDirAllSecure(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0755)
	}
	return nil
}

// GenerateBinShim creates wrapper scripts (.cmd and .ps1) for Windows to allow
// executing JavaScript binaries without the 'node' prefix.
func GenerateBinShim(binDir, name, absTarget string) error {
	if runtime.GOOS != "windows" {
		return nil
	}

	// 1. CMD Shim
	cmdPath := filepath.Join(binDir, name+".cmd")
	cmdContent := fmt.Sprintf("@ECHO off\r\nNODE  \"%%~dp0\\%s\" %%*\r\n", absTarget)
	if err := os.WriteFile(cmdPath, []byte(cmdContent), 0644); err != nil {
		return fmt.Errorf("failed to create CMD shim: %w", err)
	}

	// 2. PowerShell Shim
	psPath := filepath.Join(binDir, name+".ps1")
	psContent := fmt.Sprintf("#!/usr/bin/env pwsh\nnode \"$PSScriptRoot/%s\" $args\n", absTarget)
	if err := os.WriteFile(psPath, []byte(psContent), 0644); err != nil {
		return fmt.Errorf("failed to create PS1 shim: %w", err)
	}

	return nil
}
