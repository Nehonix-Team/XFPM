/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [script]",
	Short: "Run a script defined in package.json",
	Aliases: []string{"r"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("no script specified")
		}

		scriptName := args[0]
		projectRoot, _ := os.Getwd()

		pkgPath := filepath.Join(projectRoot, "package.json")
		pkg, err := core.LoadPackageJson(pkgPath)
		if err != nil {
			return err
		}

		if scriptCmd, ok := pkg.Scripts[scriptName]; ok {
			utils.Info("Running script: %s", scriptName)
			return executeShell(scriptCmd, projectRoot)
		}

		// If not in scripts, check if it's a file
		if _, err := os.Stat(scriptName); err == nil {
			ext := filepath.Ext(scriptName)
			if ext == ".ts" || ext == ".js" {
				// Use bun if available
				return executeCommand("bun", []string{"run", scriptName}, projectRoot)
			}
			return executeShell(scriptName, projectRoot)
		}

		return fmt.Errorf("script '%s' not found", scriptName)
	},
}

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Alias for 'xfpm run dev'",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCmd.RunE(cmd, []string{"dev"})
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(devCmd)
}

func executeShell(command, dir string) error {
	cmd := utils.GetShellCommandRaw(command)
	cmd.Dir = dir
	cmd.Env = buildRunEnv(dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func executeCommand(name string, args []string, dir string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = buildRunEnv(dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func buildRunEnv(dir string) []string {
	path := os.Getenv("PATH")
	
	// Add project node_modules/.bin
	binPath := filepath.Join(dir, "node_modules", ".bin")
	if _, err := os.Stat(binPath); err == nil {
		path = binPath + string(os.PathListSeparator) + path
	}

	// Add global XPM bin
	globalBin := paths.BinDir()
	if _, err := os.Stat(globalBin); err == nil {
		path = globalBin + string(os.PathListSeparator) + path
	}

	return utils.FormatPathEnv(os.Environ(), path)
}
