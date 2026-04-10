package core

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/xfpm-go/internal/utils"
)

type ScriptTask struct {
	PackageName    string
	PackageVersion string
	PackageDir     string
	ScriptType     string // "preinstall", "install", "postinstall"
	ScriptCommand  string
	Dependencies   []string
}

type ScriptRunner struct {
	maxParallel int
	timeout     time.Duration
	projectRoot string
}

func NewScriptRunner(projectRoot string) *ScriptRunner {
	maxParallel := runtime.NumCPU()
	if maxParallel < 4 {
		maxParallel = 4
	}
	return &ScriptRunner{
		maxParallel: maxParallel,
		timeout:     5 * time.Minute,
		projectRoot: projectRoot,
	}
}

func (r *ScriptRunner) ExecuteParallel(ctx context.Context, tasks []ScriptTask) error {
	if len(tasks) == 0 {
		return nil
	}

	fmt.Printf("\n  [SCRIPTS] Executing %d tasks...\n", len(tasks))

	sem := make(chan struct{}, r.maxParallel)
	var wg sync.WaitGroup

	// Simple grouping for execution (lifecycle order)
	for _, task := range tasks {
		wg.Add(1)
		go func(t ScriptTask) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				utils.Log("⚙", fmt.Sprintf("Running %s for %s@%s...", t.ScriptType, t.PackageName, t.PackageVersion))
				if err := r.executeSandboxed(ctx, t); err != nil {
					// Non-fatal: script failure is logged but does not abort installation (same as Rust)
					utils.Error("%s@%s → %s failed: %v", t.PackageName, t.PackageVersion, t.ScriptType, err)
				} else {
					utils.Success("%s@%s → %s success", t.PackageName, t.PackageVersion, t.ScriptType)
				}
			case <-ctx.Done():
				return
			}
		}(task)
	}

	wg.Wait()
	return nil
}

func (r *ScriptRunner) executeSandboxed(ctx context.Context, task ScriptTask) error {
	pathVal := r.buildPath(task.PackageDir)

	cmd := exec.CommandContext(ctx, "sh", "-c", task.ScriptCommand)
	cmd.Dir = task.PackageDir
	cmd.Env = r.buildEnv(pathVal)

	pr, pw, _ := os.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		return fmt.Errorf("failed to spawn script process: %w", err)
	}

	pw.Close()

	// Stream output
	go r.streamLines(pr, "│")

	err := cmd.Wait()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("script timed out after %v", r.timeout)
		}
		return err
	}

	return nil
}

func (r *ScriptRunner) buildPath(packageDir string) string {
	paths := []string{}

	// 1. Package's .bin
	packageBin := filepath.Join(filepath.Dir(packageDir), ".bin")
	if _, err := os.Stat(packageBin); err == nil {
		paths = append(paths, packageBin)
	}

	// 2. Project's .bin
	projectBin := filepath.Join(r.projectRoot, "node_modules", ".bin")
	if _, err := os.Stat(projectBin); err == nil {
		paths = append(paths, projectBin)
	}

	// 3. Global XPM bin
	if home, err := os.UserHomeDir(); err == nil {
		globalBin := filepath.Join(home, ".xpm", "bin")
		if _, err := os.Stat(globalBin); err == nil {
			paths = append(paths, globalBin)
		}
	}

	// 4. System PATH
	paths = append(paths, os.Getenv("PATH"))

	return strings.Join(paths, string(os.PathListSeparator))
}

func (r *ScriptRunner) buildEnv(pathVal string) []string {
	env := os.Environ()
	newEnv := []string{}

	// Keep existing vars but override key ones
	overrides := map[string]string{
		"PATH":                           pathVal,
		"NODE_ENV":                       "production",
		"CI":                             "true",
		"npm_config_foreground_scripts":  "true",
		"NODE_PATH":                      filepath.Join(r.projectRoot, "node_modules"),
		"TMPDIR":                         filepath.Join(r.projectRoot, "node_modules", ".xpm", "tmp"),
		"TEMP":                           filepath.Join(r.projectRoot, "node_modules", ".xpm", "tmp"),
		"TMP":                            filepath.Join(r.projectRoot, "node_modules", ".xpm", "tmp"),
	}

	// Ensure the temp dir exists
	os.MkdirAll(overrides["TMPDIR"], 0755)

	for _, e := range env {
		pair := strings.SplitN(e, "=", 2)
		if _, ok := overrides[pair[0]]; !ok {
			newEnv = append(newEnv, e)
		}
	}

	for k, v := range overrides {
		newEnv = append(newEnv, k+"="+v)
	}

	return newEnv
}

func (r *ScriptRunner) streamLines(rdr io.ReadCloser, prefix string) {
	if rdr == nil {
		return
	}
	defer rdr.Close()
	scanner := bufio.NewScanner(rdr)
	for scanner.Scan() {
		fmt.Printf("   %s %s\n", utils.DimColor.Sprint(prefix), scanner.Text())
	}
}
