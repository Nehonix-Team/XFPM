/***************************************************************************
 * XFPM (XyPriss Fast Package Manager)
 *
 * @author Nehonix
 * @license Nehonix OSL (NOSL)
 ***************************************************************************** */

package init

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

const InitScriptURL = "https://raw.githubusercontent.com/Nehonix-Team/xypriss-templates/main/out/init.xru"

// RunInit delegates project initialization directly to the XRU engine.
func RunInit(args []string) error {
	xruBin := findXruBinary()
	if xruBin == "" {
		return fmt.Errorf("XRU engine not found. Ensure 'xru' is in your PATH or installed in %s", paths.BinDir())
	}

	initScript, err := ensureInitScript()
	if err != nil {
		return fmt.Errorf("failed to load init script: %w", err)
	}

	xruArgs := append([]string{"run", initScript}, args...)
	cmd := exec.Command(xruBin, xruArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func findXruBinary() string {
	if p, err := exec.LookPath("xru"); err == nil {
		return p
	}
	binName := "xru"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	xfpmBin := filepath.Join(paths.BinDir(), binName)
	if _, err := os.Stat(xfpmBin); err == nil {
		return xfpmBin
	}
	return ""
}

func ensureInitScript() (string, error) {
	cacheDir := paths.GlobalCacheDir()
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", err
	}

	scriptPath := filepath.Join(cacheDir, "init.xru")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		utils.Log("!", "Fetching initialization orchestrator from GitHub...")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(InitScriptURL)
		if err != nil {
			return "", fmt.Errorf("failed to download init.xru: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
		}

		out, err := os.Create(scriptPath)
		if err != nil {
			return "", err
		}
		defer out.Close()

		if _, err := io.Copy(out, resp.Body); err != nil {
			return "", err
		}
	}

	return scriptPath, nil
}
