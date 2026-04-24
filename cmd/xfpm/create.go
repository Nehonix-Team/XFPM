/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [initializer]",
	Short: "Create a new project from an initializer (e.g., vite, next-app)",
	Long:  "Native implementation of project initializers. Resolves, installs and executes generators directly.",
	DisableFlagParsing: true,
	SilenceUsage: true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no initializer specified. Example: xfpm create vite")
		}

		initializer := args[0]
		remainingArgs := args[1:]

		// 1. Normalize name (e.g., vite -> create-vite)
		pkgName := initializer
		version := "latest"
		
		lastAt := strings.LastIndex(pkgName, "@")
		if lastAt > 0 { // > 0 ensures it's not the first character (scoped package)
			version = pkgName[lastAt+1:]
			pkgName = pkgName[:lastAt]
		}

		if !strings.HasPrefix(pkgName, "create-") && !strings.Contains(pkgName, "/create-") {
			if strings.Contains(pkgName, "/") {
				scopeParts := strings.SplitN(pkgName, "/", 2)
				pkgName = scopeParts[0] + "/create-" + scopeParts[1]
			} else {
				pkgName = "create-" + pkgName
			}
		}

		utils.Matrix(fmt.Sprintf("Spinning up initializer: %s", pkgName))

		// 2. Setup internal components
		xpmStore := paths.StorageDir()

		cas, err := core.NewCas(xpmStore)
		if err != nil {
			return err
		}

		// Use a dedicated ephemeral root for creates
		tempRoot := paths.EphemeralCacheDir()
		os.MkdirAll(tempRoot, 0755)

		registry := core.NewRegistryClient("", 3)
		registry.SetCacheDir(paths.RegistryCacheDir(tempRoot))
		
		resolver := core.NewResolver(registry, cas)
		
		// 3. Resolve
		rootDeps := map[string]string{pkgName: version}
		resolved, rootVersions, err := resolver.ResolveTree(context.Background(), rootDeps)
		if err != nil {
			return fmt.Errorf("resolution failed: %w", err)
		}

		// 4. Install
		installer := core.NewInstaller(cas, registry, tempRoot)
		installer.DirectDeps = rootVersions
		if err := installer.Install(context.Background(), resolved); err != nil {
			return fmt.Errorf("installation failed: %w", err)
		}

		// 5. Locate binary and binary runtime
		// We expect exactly one direct package
		realVersion := rootVersions[pkgName]
		pkgVStoreName := strings.ReplaceAll(pkgName, "/", "+") + "@" + realVersion
		pkgDir := filepath.Join(paths.LocalVStoreDir(tempRoot), pkgVStoreName, "node_modules", pkgName)

		pkgJson, err := core.LoadPackageJson(filepath.Join(pkgDir, "package.json"))
		if err != nil {
			return fmt.Errorf("failed to load package.json for %s: %w", pkgName, err)
		}

		binPath := ""
		if pkgJson.Bin != nil {
			// Handle bin being a string or a map
			var binMap map[string]string
			data, _ := json.Marshal(pkgJson.Bin)
			if err := json.Unmarshal(data, &binMap); err == nil {
				// Take first or search for name match
				for _, p := range binMap {
					binPath = filepath.Join(pkgDir, p)
					break
				}
			} else {
				var binStr string
				if err := json.Unmarshal(data, &binStr); err == nil {
					binPath = filepath.Join(pkgDir, binStr)
				}
			}
		}

		if binPath == "" {
			return fmt.Errorf("no binary found in package %s", pkgName)
		}

		utils.Info("Executing initializer...")

		// Determine runner
		runner := ""
		isJS := strings.HasSuffix(binPath, ".js") || strings.HasSuffix(binPath, ".mjs") || strings.HasSuffix(binPath, ".cjs")
		
		if isJS {
			if _, err := exec.LookPath("bun"); err == nil {
				runner = "bun"
			} else if _, err := exec.LookPath("node"); err == nil {
				runner = "node"
			} else {
				return fmt.Errorf("no JS runtime (bun or node) found to execute the initializer")
			}
		}

		var runCmd *exec.Cmd
		if runner != "" {
			fullArgs := append([]string{binPath}, remainingArgs...)
			runCmd = exec.Command(runner, fullArgs...)
		} else {
			runCmd = exec.Command(binPath, remainingArgs...)
		}

		runCmd.Stdout = os.Stdout
		runCmd.Stderr = os.Stderr
		runCmd.Stdin = os.Stdin
		runCmd.Dir, _ = os.Getwd()

		// Inject PATH so the generator can find bun/node/npx if needed
		env := os.Environ()
		env = append(env, "npm_config_user_agent=xfpm/"+utils.BinVersion)
		runCmd.Env = env

		if err := runCmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			return fmt.Errorf("execution failed: %w", err)
		}
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}
