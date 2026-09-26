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
	"strings"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
	libport "github.com/NEHONIX/libPort"
	libproc "github.com/NEHONIX/libProc"
	libxess "github.com/Nehonix-Team/libXESS"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [script]",
	Short: "Run a script defined in package.json",
	Aliases: []string{"r"},
	SilenceUsage:  true,
	SilenceErrors: true,
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
			runErr := executeShell(scriptCmd, projectRoot, projectRoot)
			if runErr != nil && isInterrupted(runErr) {
				return nil
			}
			return runErr
		}

		// If not in scripts, check if it's a file
		if _, err := os.Stat(scriptName); err == nil {
			var runErr error
			targetProjectDir := resolveTargetProjectDir(scriptName, projectRoot)
			ext := filepath.Ext(scriptName)
			if ext == ".ts" || ext == ".js" {
				runErr = executeCommand("bun", []string{"run", scriptName}, projectRoot, targetProjectDir)
			} else {
				runErr = executeShell(scriptName, projectRoot, targetProjectDir)
			}
			if runErr != nil && isInterrupted(runErr) {
				return nil
			}
			return runErr
		}

		return fmt.Errorf("script '%s' not found", scriptName)
	},
}

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Alias for 'xfpm run dev'",
	SilenceUsage:  true,
	SilenceErrors: true,
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
	SilenceUsage:  true,
	SilenceErrors: true,
	Annotations: map[string]string{
		"requireRuntime": "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCmd.RunE(cmd, []string{"start"})
	},
}

func isInterrupted(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "terminated by") || strings.Contains(msg, "interrupt") || strings.Contains(msg, "killed")
}

func resolveTargetProjectDir(targetFile, defaultDir string) string {
	absTarget, err := filepath.Abs(targetFile)
	if err != nil {
		return defaultDir
	}
	searchDir := filepath.Dir(absTarget)
	for {
		if _, err := os.Stat(filepath.Join(searchDir, "package.json")); err == nil {
			return searchDir
		}
		if _, err := os.Stat(filepath.Join(searchDir, ".env")); err == nil {
			return searchDir
		}
		parent := filepath.Dir(searchDir)
		if parent == searchDir {
			break
		}
		searchDir = parent
	}
	return defaultDir
}

func init() {
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(devCmd)
	RootCmd.AddCommand(startCmd)
}

func executeShell(command, workDir, projDir string) error {
	sess, err := libproc.CreateSession(projDir)
	if err != nil {
		return fmt.Errorf("failed to allocate session: %w", err)
	}

	baseEnv := buildRunEnv(workDir, sess)
	envPath := filepath.Join(projDir, ".env")

	var onShutdownList []func()
	onShutdown := func() {
		for _, fn := range onShutdownList {
			fn()
		}
	}

	// Host-privileged libPort supervisor for port arbitration inside sessionDir
	portSock := filepath.Join(sess.Dir, "libport.sock")
	portSup := libport.NewSupervisor(projDir, portSock)
	if err := portSup.Start(); err == nil {
		defer portSup.Stop()
		onShutdownList = append(onShutdownList, portSup.Stop)
	}

	if _, err := os.Stat(envPath); err == nil && !libxess.IsActive() {
		var shellCmd []string
		if runtime.GOOS == "windows" {
			shellCmd = []string{"cmd.exe", "/c", command}
		} else {
			shellCmd = []string{"sh", "-c", command}
		}

		sup, err := libxess.NewSupervisor(libxess.Config{
			ProjectDir:  projDir,
			EnvFileName: ".env",
			Command:     shellCmd,
			BaseEnv:     baseEnv,
			TempDir:     filepath.Join(sess.Dir, "xess"),
		})
		if err == nil {
			cmd, err := sup.Start()
			if err == nil {
				cmd.Dir = workDir
				defer sup.Stop()
				onShutdownList = append(onShutdownList, sup.Stop)
				utils.Success("libXESS loaded (Bipolar Zero-Trust Confinement)")
				return runProcess(cmd, sess.Dir, onShutdown)
			}
			utils.Warn("libXESS confinement fallback: %v", err)
		}
	}

	cmd := utils.GetShellCommandRaw(command)
	cmd.Dir = workDir
	cmd.Env = baseEnv
	return runProcess(cmd, sess.Dir, onShutdown)
}

func executeCommand(name string, args []string, workDir, projDir string) error {
	sess, err := libproc.CreateSession(projDir)
	if err != nil {
		return fmt.Errorf("failed to allocate session: %w", err)
	}

	baseEnv := buildRunEnv(workDir, sess)
	envPath := filepath.Join(projDir, ".env")

	var onShutdownList []func()
	onShutdown := func() {
		for _, fn := range onShutdownList {
			fn()
		}
	}

	// Host-privileged libPort supervisor for port arbitration inside sessionDir
	portSock := filepath.Join(sess.Dir, "libport.sock")
	portSup := libport.NewSupervisor(projDir, portSock)
	if err := portSup.Start(); err == nil {
		defer portSup.Stop()
		onShutdownList = append(onShutdownList, portSup.Stop)
	}

	if _, err := os.Stat(envPath); err == nil && !libxess.IsActive() {
		fullCmd := append([]string{name}, args...)
		sup, err := libxess.NewSupervisor(libxess.Config{
			ProjectDir:  projDir,
			EnvFileName: ".env",
			Command:     fullCmd,
			BaseEnv:     baseEnv,
			TempDir:     filepath.Join(sess.Dir, "xess"),
		})
		if err == nil {
			cmd, err := sup.Start()
			if err == nil {
				cmd.Dir = workDir
				defer sup.Stop()
				onShutdownList = append(onShutdownList, sup.Stop)
				utils.Success("libXESS loaded (Bipolar Zero-Trust Confinement)")
				return runProcess(cmd, sess.Dir, onShutdown)
			}
			utils.Warn("libXESS confinement fallback: %v", err)
		}
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	cmd.Env = baseEnv
	return runProcess(cmd, sess.Dir, onShutdown)
}

func runProcess(cmd *exec.Cmd, sessionDir string, onShutdown func()) error {
	sigChan := utils.SignalManager.Subscribe()
	defer utils.SignalManager.Unsubscribe(sigChan)

	return libproc.RunCmd(cmd, libproc.RunOptions{
		SessionDir:      sessionDir,
		GracefulTimeout: 1500 * time.Millisecond,
		StopSignal:      sigChan,
		OnShutdown:      onShutdown,
	})
}

func buildRunEnv(dir string, sess *libproc.Session) []string {
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
	xessDir := filepath.Join(sess.Dir, "xess")
	portSock := filepath.Join(sess.Dir, "libport.sock")

	env = append(env, "XFPM_VERSION="+utils.BinVersion)
	env = append(env, "XYPRISS_SESSION_HASH="+sess.Hash)
	env = append(env, "XYPRISS_USER_TMP="+sess.Dir)
	env = append(env, "XESS_SESSION_TMP="+sess.Dir)
	env = append(env, "XESS_TEMP_DIR="+xessDir)
	env = append(env, "LIBPORT_SOCKET_PATH="+portSock)
	env = append(env, "PORT_SOCKET_PATH="+portSock)
	env = append(env, "XPM_SOCKET_PATH="+portSock)
	return env
}
