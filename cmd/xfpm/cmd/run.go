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
	SilenceUsage:      true,
	SilenceErrors:     true,
	DisableFlagParsing: true,
	Annotations: map[string]string{
		"requireRuntime": "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		var runArgs []string
		forceXess := false
		disableXess := false

		for _, arg := range args {
			if arg == "--xess" || arg == "-xess" || arg == "--shield" || arg == "-shield" {
				forceXess = true
			} else if arg == "--no-xess" || arg == "-no-xess" || arg == "--no-shield" || arg == "-no-shield" {
				disableXess = true
			} else {
				runArgs = append(runArgs, arg)
			}
		}

		if len(runArgs) == 0 {
			return fmt.Errorf("no script specified")
		}

		scriptName := runArgs[0]
		projectRoot, _ := os.Getwd()

		pkgPath := filepath.Join(projectRoot, "package.json")
		pkg, err := core.LoadPackageJson(pkgPath)
		if err != nil {
			return err
		}

		opts := execOptions{
			ForceXess:   forceXess,
			DisableXess: disableXess,
		}

		if scriptCmd, ok := pkg.Scripts[scriptName]; ok {
			utils.Info("Running script: %s", scriptName)
			scriptOpts := opts
			if (scriptName == "dev" || scriptName == "start") && !opts.DisableXess {
				scriptOpts.ForceXess = true
			}
			fullCmd := scriptCmd
			if len(runArgs) > 1 {
				fullCmd = fullCmd + " " + strings.Join(runArgs[1:], " ")
			}
			runErr := executeShellWithOptions(fullCmd, projectRoot, projectRoot, scriptOpts)
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
				runErr = executeCommandWithOptions("bun", []string{"run", scriptName}, projectRoot, targetProjectDir, opts)
			} else {
				runErr = executeShellWithOptions(scriptName, projectRoot, targetProjectDir, opts)
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

func isWatcherOrMetaCommand(cmdStr string) bool {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return false
	}
	for i, f := range fields {
		base := strings.ToLower(filepath.Base(f))
		if base == "fileonix" || base == "nodemon" || base == "chokidar" || base == "watch" {
			return true
		}
		if (base == "xfpm" || base == "xfpm.exe") && i+1 < len(fields) && (fields[i+1] == "exec" || fields[i+1] == "x") {
			return true
		}
	}
	return false
}

type execOptions struct {
	ForceXess   bool
	DisableXess bool
}

func isServerEntryPoint(filePath string) bool {
	clean := filepath.ToSlash(strings.ToLower(filePath))
	// Exclude scripts directory, tools, tasks, migrations, seeds, test
	parts := strings.Split(clean, "/")
	for _, p := range parts {
		if p == "scripts" || p == "script" || p == "tools" || p == "tasks" || p == "seeds" || p == "seed" || p == "migrations" || p == "migration" || p == "tests" || p == "test" {
			return false
		}
	}
	base := filepath.Base(clean)
	// Server entrypoint files: server.ts, index.ts, app.ts, main.ts
	return base == "server.ts" || base == "server.js" ||
		base == "index.ts" || base == "index.js" ||
		base == "app.ts" || base == "app.js" ||
		base == "main.ts" || base == "main.js"
}

func isScriptTarget(name string, args []string) bool {
	if isServerEntryPoint(name) {
		return true
	}
	base := strings.ToLower(filepath.Base(name))
	if base == "bun" || base == "bun.exe" || base == "node" || base == "node.exe" || base == "tsx" || base == "deno" || base == "xfpm" || base == "xfpm.exe" {
		for _, a := range args {
			if isServerEntryPoint(a) {
				return true
			}
		}
	}
	return false
}

func isShellScriptTarget(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if isServerEntryPoint(f) {
			return true
		}
	}
	return false
}

func shouldActivateXESS(targetName string, args []string, command string, opts execOptions) bool {
	if opts.DisableXess {
		return false
	}
	if opts.ForceXess {
		return true
	}
	if command != "" {
		return isShellScriptTarget(command)
	}
	return isScriptTarget(targetName, args)
}

func executeShell(command, workDir, projDir string) error {
	return executeShellWithOptions(command, workDir, projDir, execOptions{})
}

func executeShellWithOptions(command, workDir, projDir string, opts execOptions) error {
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
	if !sess.IsAdopted {
		portSup := libport.NewSupervisor(projDir, portSock)
		if err := portSup.Start(); err == nil {
			defer portSup.Stop()
			onShutdownList = append(onShutdownList, portSup.Stop)
		}
	}

	if _, err := os.Stat(envPath); err == nil && !libxess.IsActive() && !sess.IsAdopted && !isWatcherOrMetaCommand(command) && shouldActivateXESS("", nil, command, opts) {
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
				return runProcessWithOptions(cmd, sess, onShutdown, true)
			}
			utils.Warn("libXESS confinement fallback: %v", err)
		}
	}

	cmd := utils.GetShellCommandRaw(command)
	cmd.Dir = workDir
	cmd.Env = baseEnv
	return runProcess(cmd, sess, onShutdown)
}

func executeCommand(name string, args []string, workDir, projDir string) error {
	return executeCommandWithOptions(name, args, workDir, projDir, execOptions{})
}

func executeCommandWithOptions(name string, args []string, workDir, projDir string, opts execOptions) error {
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
	if !sess.IsAdopted {
		portSup := libport.NewSupervisor(projDir, portSock)
		if err := portSup.Start(); err == nil {
			defer portSup.Stop()
			onShutdownList = append(onShutdownList, portSup.Stop)
		}
	}

	if _, err := os.Stat(envPath); err == nil && !libxess.IsActive() && !sess.IsAdopted && !isWatcherOrMetaCommand(name) && shouldActivateXESS(name, args, "", opts) {
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
				return runProcessWithOptions(cmd, sess, onShutdown, true)
			}
			utils.Warn("libXESS confinement fallback: %v", err)
		}
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = workDir
	cmd.Env = baseEnv
	return runProcess(cmd, sess, onShutdown)
}

func runProcess(cmd *exec.Cmd, sess *libproc.Session, onShutdown func()) error {
	return runProcessWithOptions(cmd, sess, onShutdown, false)
}

func runProcessWithOptions(cmd *exec.Cmd, sess *libproc.Session, onShutdown func(), isConfined bool) error {
	sigChan := utils.SignalManager.Subscribe()
	defer utils.SignalManager.Unsubscribe(sigChan)

	// If the process is not confined by libXESS supervisor, let it share the terminal process group
	// so interactive commands (like prompts, readline, curses) receive terminal input and signals cleanly.
	inheritGroup := !isConfined

	return libproc.RunCmd(cmd, libproc.RunOptions{
		SessionDir:          sess.Dir,
		IsAdopted:           sess.IsAdopted,
		GracefulTimeout:     1500 * time.Millisecond,
		InheritProcessGroup: inheritGroup,
		StopSignal:          sigChan,
		OnShutdown:          onShutdown,
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
