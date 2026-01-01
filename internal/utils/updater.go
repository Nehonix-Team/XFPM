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
		Success("XFPM updated successfully! Please restart your terminal.")
		pterm.Println()
		os.Exit(0)
	}
}
