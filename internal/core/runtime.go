/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

// EnsureRuntime checks if the required JS runtime (Bun) is available.
func EnsureRuntime() error {
	// First, check if we need to migrate from .xpm to .xfpm
	MigrateHome()

	binDir := paths.BinDir()
	bunName := "bun"
	if runtime.GOOS == "windows" {
		bunName = "bun.exe"
	}

	installedPath, lookErr := exec.LookPath(bunName)
	
	// Case 1: Already managed by XFPM
	if lookErr == nil && strings.HasPrefix(installedPath, binDir) {
		return nil
	}

	// Case 2: Found elsewhere on the system
	if lookErr == nil {
		utils.Premium("RUNTIME", fmt.Sprintf("Bun is installed at %s but is not managed by XFPM.", installedPath))
		pterm.Printf("   %s %s\n", 
			pterm.FgYellow.Sprint("?"), 
			pterm.Bold.Sprint("For some reasons, we need to uninstall your actual bun binary to reinstall it manually; do u agree? (y/N)"))
	} else {
		// Case 3: Not found at all
		utils.Premium("RUNTIME", "XyPriss requires the Bun runtime to execute scripts and servers.")
		pterm.Printf("   %s %s\n", 
			pterm.FgYellow.Sprint("?"), 
			pterm.Bold.Sprint("Would you like XFPM to automatically install Bun for you? (y/N)"))
	}
	
	fmt.Printf("   %s ", pterm.FgCyan.Sprint(">"))
	
	var input string
	fmt.Scanln(&input)

	if strings.ToLower(input) != "y" {
		if lookErr == nil {
			utils.Info("Using existing Bun at %s. Some features may be unstable.", installedPath)
			return nil
		}
		utils.Warn("XyPriss may not function correctly without Bun. You can install it later with 'xfpm i -g bun'.")
		return nil
	}

	return DownloadAndInstallBun()
}

// DownloadAndInstallBun leverages XFPM's internal resolver and installer
// to perform a silent global installation of @oven/bun.
func DownloadAndInstallBun() error {
	ctx := context.Background()
	
	// 1. Setup Environment
	projectRoot := paths.GlobalsDir()
	utils.CreateDirAllSecure(projectRoot)

	xpmStore := paths.StorageDir()

	// 2. Initialize Internal Stack
	cas, err := NewCas(xpmStore)
	if err != nil {
		return fmt.Errorf("failed to init storage: %w", err)
	}

	xpmDir := paths.LocalXpmDir(projectRoot)
	registry := NewRegistryClient("", 3)
	registry.SetCacheDir(paths.RegistryCacheDir(xpmDir))

	utils.Info("Bootstrapping Bun runtime via XFPM engine...")

	// 3. Resolve @oven/bun
	resolver := NewResolver(registry, cas)
	rootDeps := map[string]string{"bun": "latest"}
	
	resolved, rootVersions, err := resolver.ResolveTree(ctx, rootDeps)
	if err != nil {
		return fmt.Errorf("failed to resolve bun: %w", err)
	}

	// 4. Install
	installer := NewInstaller(cas, registry, projectRoot)
	installer.IsGlobal = true
	installer.DirectDeps = rootVersions
	installer.NoInteract = true // Silent for bootstrapping

	if err := installer.Install(ctx, resolved); err != nil {
		return fmt.Errorf("failed to install bun: %w", err)
	}

	utils.Success("Bun runtime successfully integrated into XyPriss.")
	return nil
}

// RemoveRuntime deletes the Bun runtime from the global .xpm/bin directory
func RemoveRuntime() error {
	binDir := paths.BinDir()
	
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

// MigrateHome handles the transition from the old .xpm directory to the new .xfpm standard.
// It performs a simple directory rename if the old directory exists and the new one doesn't.
func MigrateHome() {
	home, _ := os.UserHomeDir()
	oldXpm := filepath.Join(home, ".xpm")
	newXfpm := paths.XpmHome()

	if _, err := os.Stat(newXfpm); os.IsNotExist(err) {
		if _, err := os.Stat(oldXpm); err == nil {
			utils.Info("Detected legacy home directory: %s", oldXpm)
			utils.Info("Migrating to new standard: %s...", newXfpm)
			if err := os.Rename(oldXpm, newXfpm); err != nil {
				utils.Error("Failed to auto-migrate home directory: %v", err)
			} else {
				utils.Success("Home directory migrated successfully.")
			}
		}
	}
}
