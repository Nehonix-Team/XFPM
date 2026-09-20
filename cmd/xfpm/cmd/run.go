/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 * @license Nehonix OSL (NOSL)
 * Copyright (c) 2025 Nehonix. All rights reserved.
 ***************************************************************************** */

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	libxess "github.com/Nehonix-Team/libXESS"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [script]",
	Short: "Run a script defined in package.json",
	Aliases: []string{"r"},
	Annotations: map[string]string{
		"requireRuntime": "true",
	},
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
	Annotations: map[string]string{
		"requireRuntime": "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCmd.RunE(cmd, []string{"dev"})
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Alias for 'xfpm run start'",
	Annotations: map[string]string{
		"requireRuntime": "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCmd.RunE(cmd, []string{"start"})
	},
}

func init() {
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(devCmd)
	RootCmd.AddCommand(startCmd)
}

func executeShell(command, dir string) error {
	baseEnv := buildRunEnv(dir)
	envPath := filepath.Join(dir, ".env")

	if _, err := os.Stat(envPath); err == nil {
		var shellCmd []string
		if runtime.GOOS == "windows" {
			shellCmd = []string{"cmd.exe", "/c", command}
		} else {
			shellCmd = []string{"sh", "-c", command}
		}

		sup, err := libxess.NewSupervisor(libxess.Config{
			ProjectDir:  dir,
			EnvFileName: ".env",
			Command:     shellCmd,
			BaseEnv:     baseEnv,
			TempDir:     libxess.DefaultTempDir(),
		})
		if err == nil {
			cmd, err := sup.Start()
			if err == nil {
				defer sup.Stop()
				utils.Success("libXESS loaded (Bipolar Zero-Trust Confinement)")
				return runProcess(cmd)
			}
			utils.Warn("libXESS confinement fallback: %v", err)
		}
	}

	cmd := utils.GetShellCommandRaw(command)
	cmd.Dir = dir
	cmd.Env = baseEnv
	return runProcess(cmd)
}

func executeCommand(name string, args []string, dir string) error {
	baseEnv := buildRunEnv(dir)
	envPath := filepath.Join(dir, ".env")

	if _, err := os.Stat(envPath); err == nil {
		fullCmd := append([]string{name}, args...)
		sup, err := libxess.NewSupervisor(libxess.Config{
			ProjectDir:  dir,
			EnvFileName: ".env",
			Command:     fullCmd,
			BaseEnv:     baseEnv,
			TempDir:     libxess.DefaultTempDir(),
		})
		if err == nil {
			cmd, err := sup.Start()
			if err == nil {
				defer sup.Stop()
				utils.Success("libXESS loaded (Bipolar Zero-Trust Confinement)")
				return runProcess(cmd)
			}
			utils.Warn("libXESS confinement fallback: %v", err)
		}
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = baseEnv
	return runProcess(cmd)
}

func runProcess(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	sigChan := utils.SignalManager.Subscribe()
	defer utils.SignalManager.Unsubscribe(sigChan)

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		select {
		case sig, ok := <-sigChan:
			if ok && sig != nil && cmd.Process != nil {
				_ = cmd.Process.Signal(sig)
			}
		case <-done:
		}
	}()

	err := cmd.Wait()
	close(done)
	return err
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

	env := utils.FormatPathEnv(os.Environ(), path)
	tempDir := libxess.DefaultTempDir()
	env = append(env, "XFPM_VERSION="+utils.BinVersion)
	env = append(env, "XESS_TEMP_DIR="+tempDir)
	env = append(env, "XYPRISS_USER_TMP="+filepath.Dir(tempDir))
	return env
}
