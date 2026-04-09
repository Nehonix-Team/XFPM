/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/pterm/pterm"
)

const (
	VersionCheckURL  = "https://raw.githubusercontent.com/Nehonix-Team/XFMP/master/version.json"
	InstallScriptURL = "https://xypriss.nehonix.com/install.js"
)

// CheckForUpdates fetches the latest version from Nehonix GitHub and proposes an update if needed.
func CheckForUpdates() {
	// Skip if explicitly disabled or in a CI environment
	if os.Getenv("XPM_NO_UPDATE") != "" || os.Getenv("CI") != "" {
		return
	}

	// Use a background check would be better but the user asked for it at launch.
	// We use a very short timeout to avoid blocking the user if network is slow.
	client := http.Client{
		Timeout: 1500 * time.Millisecond,
	}

	resp, err := client.Get(VersionCheckURL)
	if err != nil {
		return // Fail silently
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var data struct {
		Latest string `json:"latest"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return
	}

	if data.Latest != "" && data.Latest != BinVersion {
		// Only show update if the remote version is lexicographically greater (simplistic but works for G0.1.x)
		// and avoid showing if the local version is already ahead.
		if data.Latest < BinVersion {
			return 
		}

		pterm.Println()
		Premium("UPDATE", fmt.Sprintf("A new version of XFPM is available: %s (Current: %s)", 
			pterm.FgGreen.Sprint(data.Latest), 
			pterm.FgGray.Sprint(BinVersion)))
		
		pterm.Printf("   %s %s\n", 
			pterm.FgYellow.Sprint("?"), 
			pterm.Bold.Sprint("Would you like to install the update now? (y/N)"))
		
		fmt.Printf("   %s ", pterm.FgCyan.Sprint(">"))
		
		var input string
		fmt.Scanln(&input)

		if input == "y" || input == "Y" {
			PerformSelfUpdate()
		}
		pterm.Println()
	}
}

// PerformSelfUpdate runs the official installation script according to the current platform.
func PerformSelfUpdate() {
	pterm.Println()
	Info("Launching Nehonix Installer...")

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", fmt.Sprintf("Invoke-RestMethod -Uri '%s' -UseBasicParsing | node", InstallScriptURL))
	} else {
		// Use sh/bash for Unix-like systems
		cmd = exec.Command("sh", "-c", fmt.Sprintf("curl -sL %s | node", InstallScriptURL))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		pterm.Println()
		Error("Installation failed: %v", err)
		pterm.Println()
	} else {
		pterm.Println()
		Success("XFPM updated successfully! Proceeding with task...")
		pterm.Println()
		ContinueTask()
	}
}

// ContinueTask attempts to re-execute the current command using the updated binary (Unix)
// or simply returns to allow the current process to continue its task.
func ContinueTask() {
	if runtime.GOOS == "windows" {
		// On Windows, hard to re-exec in-place safely without side effects
		return
	}

	binary, err := exec.LookPath(os.Args[0])
	if err != nil {
		return 
	}

	// Ensure the new process doesn't check for updates again immediately
	os.Setenv("XPM_NO_UPDATE", "1")

	// Replace current process with new binary
	err = syscall.Exec(binary, os.Args, os.Environ())
	if err != nil {
		// Fallback: if re-exec fails, just let the current process finish the task
		return
	}
}
