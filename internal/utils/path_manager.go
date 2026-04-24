package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Nehonix-Team/XFMP/internal/paths"
)

const pathMarker = "# XFPM PATH (auto-generated)"

func EnsurePathInShell() error {
	binPath := paths.BinDir()

	switch runtime.GOOS {
	case "windows":
		return setupWindowsPath(binPath)
	case "darwin", "linux":
		home, _ := os.UserHomeDir()
		return setupUnixPath(home, binPath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// isPathActive reports whether binPath is already present in the current process PATH.
func isPathActive(binPath string) bool {
	current := os.Getenv("PATH")
	sep := string(os.PathListSeparator) // ':' on Unix, ';' on Windows
	for _, p := range strings.Split(current, sep) {
		if filepath.Clean(p) == filepath.Clean(binPath) {
			return true
		}
	}
	return false
}

// injectPathInProcess adds binPath to the current process environment.
// This only affects the running Go process and its children — not the parent shell.
func injectPathInProcess(binPath string) {
	current := os.Getenv("PATH")
	sep := string(os.PathListSeparator)
	_ = os.Setenv("PATH", binPath+sep+current)
}

// setupUnixPath writes binPath into the appropriate shell profile(s) and
// injects it into the current process immediately.
func setupUnixPath(home, binPath string) error {
	// Always inject into the current process so xpm sub-commands work right away.
	if !isPathActive(binPath) {
		injectPathInProcess(binPath)
	}

	profiles := resolveShellProfiles(home)
	exportLine := fmt.Sprintf("\n%s\nexport PATH=\"%s:$PATH\"\n", pathMarker, binPath)

	var lastErr error
	modifiedAny := false
	var modifiedProfiles []string

	for _, profile := range profiles {
		if _, err := os.Stat(profile); os.IsNotExist(err) {
			continue
		}

		content, err := os.ReadFile(profile)
		if err != nil {
			lastErr = fmt.Errorf("could not read %s: %w", profile, err)
			continue
		}

		if strings.Contains(string(content), binPath) {
			continue // Already written, skip
		}

		f, err := os.OpenFile(profile, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			lastErr = fmt.Errorf("could not open %s for writing: %w", profile, err)
			continue
		}

		_, writeErr := f.WriteString(exportLine)
		closeErr := f.Close()

		if writeErr != nil {
			lastErr = fmt.Errorf("could not write to %s: %w", profile, writeErr)
			continue
		}
		if closeErr != nil {
			lastErr = fmt.Errorf("could not close %s: %w", profile, closeErr)
			continue
		}

		modifiedAny = true
		modifiedProfiles = append(modifiedProfiles, profile)
	}

	if modifiedAny {
		// PATH is active in this process already.
		// But the parent shell (the terminal) still needs a manual source — this is
		// a hard OS limitation: a child process cannot mutate its parent's environment.
		Info("PATH injected for this session. Profile(s) updated: %s", strings.Join(modifiedProfiles, ", "))
		Info("To persist in your current terminal, run: %s", sourceHint(modifiedProfiles))
	}

	if !modifiedAny && lastErr != nil {
		return lastErr
	}

	return nil
}

// sourceHint returns the exact shell command the user should run to reload their profile.
func sourceHint(profiles []string) string {
	if len(profiles) == 0 {
		return "source ~/.profile"
	}
	// Use the first modified profile; it was already prioritized by resolveShellProfiles.
	profile := profiles[0]
	home, _ := os.UserHomeDir()

	// Replace home dir with ~ for readability
	if home != "" {
		profile = strings.Replace(profile, home, "~", 1)
	}

	shellPath := os.Getenv("SHELL")
	switch {
	case strings.Contains(shellPath, "fish"):
		// fish uses `source`, same syntax
		return fmt.Sprintf("source %s", profile)
	default:
		return fmt.Sprintf("source %s", profile)
	}
}

// resolveShellProfiles returns an ordered, deduplicated list of shell profile paths.
// The active shell's profile is prioritized first.
func resolveShellProfiles(home string) []string {
	candidates := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".profile"),
	}

	// macOS: .zprofile is the default login shell profile since Catalina
	if runtime.GOOS == "darwin" {
		candidates = append([]string{filepath.Join(home, ".zprofile")}, candidates...)
	}

	shellPath := os.Getenv("SHELL")
	var priority string
	switch {
	case strings.Contains(shellPath, "zsh"):
		if runtime.GOOS == "darwin" {
			priority = filepath.Join(home, ".zprofile")
		} else {
			priority = filepath.Join(home, ".zshrc")
		}
	case strings.Contains(shellPath, "bash"):
		priority = filepath.Join(home, ".bashrc")
	case strings.Contains(shellPath, "fish"):
		priority = filepath.Join(home, ".config", "fish", "config.fish")
		candidates = append(candidates, priority)
	}

	if priority != "" {
		deduped := []string{priority}
		for _, c := range candidates {
			if c != priority {
				deduped = append(deduped, c)
			}
		}
		return deduped
	}

	return candidates
}

// setupWindowsPath updates the user-level PATH via PowerShell (no admin required)
// and injects it into the current process immediately.
func setupWindowsPath(binPath string) error {
	// Inject into current process right away
	if !isPathActive(binPath) {
		injectPathInProcess(binPath)
	}

	// Read current user PATH from registry
	checkScript := `[Environment]::GetEnvironmentVariable("PATH", "User")`
	out, err := runPowerShell(checkScript)
	if err != nil {
		return fmt.Errorf("could not read Windows user PATH: %w", err)
	}

	normalized := filepath.ToSlash(binPath)
	if strings.Contains(filepath.ToSlash(out), normalized) {
		return nil // Already present in registry
	}

	// Persist to user registry — no admin required, takes effect for new processes
	setScript := fmt.Sprintf(
		`$cur = [Environment]::GetEnvironmentVariable("PATH", "User"); `+
			`[Environment]::SetEnvironmentVariable("PATH", $cur + ";%s", "User")`,
		binPath,
	)
	if _, err := runPowerShell(setScript); err != nil {
		return fmt.Errorf("could not update Windows user PATH: %w", err)
	}

	// Broadcast WM_SETTINGCHANGE so Explorer and new terminals pick up the change
	// without a full reboot. Errors here are non-fatal.
	broadcastScript := `
		Add-Type -TypeDefinition @"
		using System;
		using System.Runtime.InteropServices;
		public class WinEnv {
			[DllImport("user32.dll", SetLastError=true, CharSet=CharSet.Auto)]
			public static extern IntPtr SendMessageTimeout(
				IntPtr hWnd, uint Msg, UIntPtr wParam,
				string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);
		}
"@
		$r = [UIntPtr]::Zero
		[WinEnv]::SendMessageTimeout(
			[IntPtr]0xFFFF, 0x001A, [UIntPtr]::Zero,
			"Environment", 2, 5000, [ref]$r) | Out-Null
	`
	_, _ = runPowerShell(broadcastScript) // best-effort, ignore errors

	Info("PATH injected for this session and persisted to Windows user environment.")
	Info("New terminals will pick it up automatically. Running processes may need a restart.")
	return nil
}

// runPowerShell executes a PowerShell command and returns its trimmed stdout.
func runPowerShell(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}