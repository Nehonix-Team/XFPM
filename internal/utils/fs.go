package utils

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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

// LinkDir creates a symbolic link for a directory.
// On Windows, if symlinking fails (e.g., due to missing privileges), it falls back to a Directory Junction.
func LinkDir(oldname, newname string) error {
	// Ensure oldname is absolute for Junctions
	absOld := oldname
	if !filepath.IsAbs(oldname) {
		absOld, _ = filepath.Abs(filepath.Join(filepath.Dir(newname), oldname))
	}

	err := os.Symlink(oldname, newname)
	if err == nil || runtime.GOOS != "windows" {
		return err
	}

	// Fallback to Junction on Windows
	// We use 'cmd /c mklink /j'
	// Junctions MUST use absolute paths for the target to be reliable.
	cmd := exec.Command("cmd", "/c", "mklink", "/j", newname, absOld)
	if err := cmd.Run(); err != nil {
		// Final fallback: Copying (expensive but works)
		return CopyDir(absOld, newname)
	}
	return nil
}

// CopyDir recursively copies a directory.
func CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		
		// For files, we try to use hardlinks first to save space
		if err := os.Link(path, target); err == nil {
			return nil
		}

		// Fallback to copy
		s, err := os.Open(path)
		if err != nil {
			return err
		}
		defer s.Close()
		d, err := os.Create(target)
		if err != nil {
			return err
		}
		defer d.Close()
		_, err = io.Copy(d, s)
		return err
	})
}
