package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	libxess "github.com/Nehonix-Team/libXESS"
	"github.com/spf13/cobra"
)

var internalSandboxCmd = &cobra.Command{
	Use:                "internal-sandbox",
	Hidden:             true,
	SilenceUsage:       true,
	SilenceErrors:      true,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("internal-sandbox: no target command specified")
		}

		targetCmd := args
		bin, err := exec.LookPath(targetCmd[0])
		if err != nil {
			if home, hErr := os.UserHomeDir(); hErr == nil {
				candidate := filepath.Join(home, ".bun", "bin", targetCmd[0])
				if _, sErr := os.Stat(candidate); sErr == nil {
					bin = candidate
					err = nil
				}
			}
		}
		if err != nil {
			return fmt.Errorf("internal-sandbox: binary not found '%s': %w", targetCmd[0], err)
		}

		// Resolve symlinks to target the real physical binary
		if realBin, rErr := filepath.EvalSymlinks(bin); rErr == nil && realBin != "" {
			bin = realBin
		}
		targetCmd[0] = bin

		// 1. Determine Project Sandbox Directory (strictly user's project space)
		projectDir := os.Getenv("XYPRISS_PROJECT_ROOT")
		if projectDir == "" {
			if cwd, err := os.Getwd(); err == nil {
				projectDir = cwd
			}
		}

		// 2. Determine Session Directory (temporary socket IPC)
		sessionDir := os.Getenv("XYPRISS_USER_TMP")
		if sessionDir == "" {
			sessionDir = os.Getenv("XESS_SESSION_TMP")
		}

		var rwPaths []string
		if projectDir != "" {
			rwPaths = append(rwPaths, projectDir)
			_ = os.Chdir(projectDir)
		}
		if sessionDir != "" {
			rwPaths = append(rwPaths, sessionDir)
		}

		// 3. Determine Read-Only Paths (Runtime binaries, authorized plugins, workspace dependencies)
		var roPaths []string
		binDir := filepath.Dir(bin)
		roPaths = append(roPaths, binDir)
		if home, err := os.UserHomeDir(); err == nil {
			roPaths = append(roPaths, home)
			bunDir := filepath.Join(home, ".bun")
			if _, err := os.Stat(bunDir); err == nil {
				roPaths = append(roPaths, bunDir)
			}
			xfpmDir := filepath.Join(home, ".xfpm")
			if _, err := os.Stat(xfpmDir); err == nil {
				roPaths = append(roPaths, xfpmDir)
			}
		}

		// Authorize plugins from the official xfpm manifest
		manifestPath := os.Getenv("XYPRISS_PLUGINS_MANIFEST")
		if manifestPath != "" {
			roPaths = append(roPaths, filepath.Dir(manifestPath))
			if data, err := os.ReadFile(manifestPath); err == nil {
				var mf struct {
					Roots map[string]string `json:"roots"`
				}
				if json.Unmarshal(data, &mf) == nil && mf.Roots != nil {
					for root := range mf.Roots {
						roPaths = append(roPaths, root)
					}
				}
			}
		}

		// Authorize ancestor parent directories up to root (needed by getcwd(), realpath, and runtime path resolution)
		if projectDir != "" {
			curr := filepath.Dir(projectDir)
			for curr != "/" && curr != "." && curr != "" {
				roPaths = append(roPaths, curr)
				parent := filepath.Dir(curr)
				if parent == curr {
					break
				}
				curr = parent
			}
		}

		// 4. Apply Linux Landlock LSM kernel confinement
		if libxess.IsLandlockSupported() {
			if lErr := libxess.ApplyLandlock(roPaths, rwPaths); lErr != nil {
				fmt.Fprintf(os.Stderr, "[internal-sandbox warning] Landlock application error: %v\n", lErr)
			}
		}

		// 5. Replace process image cleanly with the target command (bun)
		if err := syscall.Exec(bin, targetCmd, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "[internal-sandbox error] Failed to execute '%s': %v (ro: %v, rw: %v)\n", bin, err, roPaths, rwPaths)
			return err
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(internalSandboxCmd)
}
