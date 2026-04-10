package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const pathMarker = "# XFPM PATH (auto-generated)"

func EnsurePathInShell() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	binPath := filepath.Join(home, ".xpm", "bin")

	switch runtime.GOOS {
	case "windows":
		return setupWindowsPath(binPath)
	case "darwin", "linux":
		return setupUnixPath(home, binPath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// setupUnixPath appends the bin path to the appropriate shell profile(s).
func setupUnixPath(home, binPath string) error {
	profiles := resolveShellProfiles(home)

	exportLine := fmt.Sprintf("\n%s\nexport PATH=\"%s:$PATH\"\n", pathMarker, binPath)

	var lastErr error
	modifiedAny := false

	for _, profile := range profiles {
		if _, err := os.Stat(profile); os.IsNotExist(err) {
			continue
		}

		content, err := os.ReadFile(profile) // replaces deprecated ioutil.ReadFile
		if err != nil {
			lastErr = fmt.Errorf("could not read %s: %w", profile, err)
			continue
		}

		if strings.Contains(string(content), binPath) {
			continue // Already present, skip
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
	}

	if modifiedAny {
		Info("PATH updated in your shell profile. Run 'source ~/.bashrc' or 'source ~/.zshrc' (or open a new terminal) to apply.")
	}

	// Return last encountered error only if nothing was modified at all
	if !modifiedAny && lastErr != nil {
		return lastErr
	}

	return nil
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

	// macOS-specific: .zprofile is the default login shell profile since Catalina
	if runtime.GOOS == "darwin" {
		candidates = append([]string{filepath.Join(home, ".zprofile")}, candidates...)
	}

	// Prioritize the currently active shell
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
		// Move priority to front, deduplicate
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

// setupWindowsPath updates the user-level PATH via PowerShell (no admin required).
func setupWindowsPath(binPath string) error {
	// Read current user PATH from registry via PowerShell
	checkScript := `[Environment]::GetEnvironmentVariable("PATH", "User")`
	out, err := runPowerShell(checkScript)
	if err != nil {
		return fmt.Errorf("could not read Windows user PATH: %w", err)
	}

	// Normalize separators for comparison
	normalized := filepath.ToSlash(binPath)
	if strings.Contains(filepath.ToSlash(out), normalized) {
		return nil // Already present
	}

	// Append binPath to user PATH
	setScript := fmt.Sprintf(
		`$cur = [Environment]::GetEnvironmentVariable("PATH", "User"); `+
			`[Environment]::SetEnvironmentVariable("PATH", $cur + ";%s", "User")`,
		binPath,
	)
	if _, err := runPowerShell(setScript); err != nil {
		return fmt.Errorf("could not update Windows user PATH: %w", err)
	}

	Info("PATH updated in Windows user environment. Please restart your terminal to apply.")
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