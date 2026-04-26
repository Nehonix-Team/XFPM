package utils

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens the specified URL in the default system browser.
func OpenBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf(" [!] Warning: Could not open browser: %v\n", err)
	}
}

// GetShellCommand returns an exec.Cmd configured with the appropriate shell for the current OS.
func GetShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

// GetShellCommandRaw returns an exec.Cmd without context configured with the appropriate shell.
func GetShellCommandRaw(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("sh", "-c", command)
}

// FormatPathEnv standardizes the PATH variable in an environment slice,
// ensuring case-insensitive replacement for Windows compatibility.
func FormatPathEnv(env []string, newPath string) []string {
	found := false
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			// Persist the original casing of the "PATH=" key part (e.g., "Path=" or "PATH=")
			env[i] = e[:5] + newPath
			found = true
			break
		}
	}
	if !found {
		return append(env, "PATH="+newPath)
	}
	return env
}

// CleanEnvForOverride removes variables from an environment slice that will be overridden.
// This is specifically helpful for Windows where 'Path' and 'PATH' might coexist.
func CleanEnvForOverride(env []string, overrides map[string]string) []string {
	newEnv := []string{}
	upperOverrides := make(map[string]bool)
	for k := range overrides {
		upperOverrides[strings.ToUpper(k)] = true
	}

	for _, e := range env {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) < 1 {
			continue
		}
		
		key := strings.ToUpper(pair[0])
		if !upperOverrides[key] {
			newEnv = append(newEnv, e)
		}
	}
	return newEnv
}
