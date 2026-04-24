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
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

// EnsureRuntime checks if the required JS runtime (Bun) is available.
// If missing, it prompts the user and installs it into ~/.xpm/bin/
// using XFPM's internal installation engine.
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
		utils.Warn("XyPriss may not function correctly without Bun. You can install it later with 'xfpm i -g bun'.")
		return nil
	}

	return DownloadAndInstallBun()
}

// DownloadAndInstallBun leverages XFPM's internal resolver and installer
// to perform a silent global installation of @oven/bun.
func DownloadAndInstallBun() error {
	ctx := context.Background()
	home, _ := os.UserHomeDir()
	
	// 1. Setup Environment
	projectRoot := filepath.Join(home, ".xpm", "globals")
	utils.CreateDirAllSecure(projectRoot)

	var xpmStore string
	if envStore := os.Getenv("XFPM_STORAGE"); envStore != "" {
		xpmStore = envStore
	} else {
		xpmStore = filepath.Join(home, ".xpm", "storage")
	}

	// 2. Initialize Internal Stack
	cas, err := NewCas(xpmStore)
	if err != nil {
		return fmt.Errorf("failed to init storage: %w", err)
	}

	xpmDir := filepath.Join(projectRoot, "node_modules", ".xpm")
	registry := NewRegistryClient("", 3)
	registry.SetCacheDir(filepath.Join(xpmDir, "cache"))

	utils.Info("Bootstrapping Bun runtime via XFPM engine...")

	// 3. Resolve @oven/bun
	resolver := NewResolver(registry, cas)
	rootDeps := map[string]string{"@oven/bun": "latest"}
	
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
