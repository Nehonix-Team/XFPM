/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package core

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

const (
	BunVersion = "1.1.0" // Version stable validée pour XyPriss
)

// EnsureRuntime checks if the required JS runtime (Bun) is available.
// If missing, it prompts the user and installs it into ~/.xpm/bin/
func EnsureRuntime() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	binDir := filepath.Join(home, ".xpm", "bin")
	bunName := "bun"
	if runtime.GOOS == "windows" {
		bunName = "bun.exe"
	}

	bunPath := filepath.Join(binDir, bunName)

	if _, err := os.Stat(bunPath); err == nil {
		return nil // Bun is already there
	}

	// Not found, trigger auto-install
	utils.Premium("RUNTIME", "XyPriss requires the Bun runtime to execute scripts and servers.")
	
	pterm.Printf("   %s %s\n", 
		pterm.FgYellow.Sprint("?"), 
		pterm.Bold.Sprint("Would you like XFPM to automatically install Bun for you? (y/N)"))
	
	fmt.Printf("   %s ", pterm.FgCyan.Sprint(">"))
	
	var input string
	fmt.Scanln(&input)

	if strings.ToLower(input) != "y" {
		utils.Warn("XyPriss may not function correctly without Bun. You can install it later with XFPM.")
		return nil
	}

	return DownloadAndInstallBun(binDir)
}

func DownloadAndInstallBun(targetDir string) error {
	platform, err := getBunPlatform()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://github.com/oven-sh/bun/releases/download/bun-v%s/bun-%s.zip", BunVersion, platform)
	
	utils.Info("Downloading Bun v%s for %s...", BunVersion, platform)
	
	utils.CreateDirAllSecure(targetDir)
	
	zipPath := filepath.Join(targetDir, "bun.zip")
	if err := downloadFile(url, zipPath); err != nil {
		return fmt.Errorf("failed to download bun: %w", err)
	}
	defer os.Remove(zipPath)

	utils.Info("Extracting runtime...")
	if err := unzipBun(zipPath, targetDir); err != nil {
		return fmt.Errorf("failed to unzip bun: %w", err)
	}

	// Ensure executable bit on Unix
	if runtime.GOOS != "windows" {
		os.Chmod(filepath.Join(targetDir, "bun"), 0755)
	}

	utils.Success("Bun runtime installed successfully in %s", targetDir)
	return nil
}

func getBunPlatform() (string, error) {
	osPart := runtime.GOOS
	archPart := runtime.GOARCH

	// Bun naming quirks
	if osPart == "darwin" {
		osPart = "darwin"
	}
	if archPart == "amd64" {
		archPart = "x64"
	} else if archPart == "arm64" {
		archPart = "aarch64"
	}

	// Specific baseline for older CPUs if needed, but we go with standard
	platform := fmt.Sprintf("%s-%s", osPart, archPart)
	
	// Valid platforms for Bun: 
	// darwin-aarch64, darwin-x64, linux-aarch64, linux-x64, windows-x64
	valid := map[string]bool{
		"darwin-aarch64":  true,
		"darwin-x64":      true,
		"linux-aarch64":   true,
		"linux-x64":       true,
		"windows-x64":     true,
	}

	if !valid[platform] {
		return "", fmt.Errorf("unsupported platform for Bun: %s", platform)
	}

	return platform, nil
}

func downloadFile(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func unzipBun(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Bun zips contain a nested folder like "bun-linux-x64/bun"
		// We want to extract the binary directly to the dest folder
		if strings.HasSuffix(f.Name, "/bun") || strings.HasSuffix(f.Name, "/bun.exe") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			newName := "bun"
			if strings.HasSuffix(f.Name, ".exe") {
				newName = "bun.exe"
			}
			
			path := filepath.Join(dest, newName)
			fout, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer fout.Close()

			_, err = io.Copy(fout, rc)
			if err != nil {
				return err
			}
			return nil // Found and extracted
		}
	}

	return fmt.Errorf("bun binary not found in zip")
}

// RemoveRuntime deletes the Bun runtime from the global .xpm/bin directory
func RemoveRuntime() error {
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".xpm", "bin")
	
	bunName := "bun"
	if runtime.GOOS == "windows" {
		bunName = "bun.exe"
	}

	path := filepath.Join(binDir, bunName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		utils.Info("Bun runtime is not installed.")
		return nil
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove bun: %w", err)
	}

	utils.Success("Bun runtime has been uninstalled.")
	return nil
}
