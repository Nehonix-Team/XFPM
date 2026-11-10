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

	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

type ScriptTask struct {
	PackageName    string
	PackageVersion string
	PackageDir     string
	ScriptType     string // "preinstall", "install", "postinstall"
	ScriptCommand  string
	Dependencies   []string
}

// lifecycleAllowedEnv is the strict Zero-Trust whitelist of environment variables
// permitted to reach package lifecycle scripts (preinstall, install, postinstall).
//
// Ref: Issue #52 (https://github.com/Nehonix-Team/XyPriss/issues/52)
// Supply-Chain / Postinstall Shield: Malicious packages frequently exfiltrate credentials
// during installation scripts before the application runtime starts. Any secret or unlisted
// variable from the host shell/CI runner is automatically stripped.
var lifecycleAllowedEnv = map[string]bool{
	// System identity & Shell
	"PATH":        true,
	"PWD":         true,
	"HOME":        true,
	"USER":        true,
	"LOGNAME":     true,
	"SHELL":       true,
	"TERM":        true,
	"LANG":        true,
	"LC_ALL":      true,
	"LC_CTYPE":    true,
	"COLORTERM":   true,
	"FORCE_COLOR": true,
	"TZ":          true,
	"SHLVL":       true,
	"_":           true,

	// Standard OS & Windows runtime variables
	"SYSTEMROOT":         true,
	"WINDIR":             true,
	"COMSPEC":            true,
	"PATHEXT":            true,
	"APPDATA":            true,
	"LOCALAPPDATA":       true,
	"USERPROFILE":        true,
	"ALLUSERSPROFILE":    true,
	"SYSTEMDRIVE":        true,
	"PROGRAMDATA":        true,
	"PROGRAMFILES":       true,
	"PROGRAMFILES(X86)":  true,
	"COMMONPROGRAMFILES": true,
	"HOMEDRIVE":          true,
	"HOMEPATH":           true,

	// Network proxies (required in corporate / CI / VPN environments)
	"HTTP_PROXY":  true,
	"HTTPS_PROXY": true,
	"NO_PROXY":    true,
	"http_proxy":  true,
	"https_proxy": true,
	"no_proxy":    true,

	// Build toolchains and compilers (node-gyp, C/C++, Rust, Python)
	"CC":              true,
	"CXX":             true,
	"CFLAGS":          true,
	"CXXFLAGS":        true,
	"CPPFLAGS":        true,
	"LDFLAGS":         true,
	"MAKE":            true,
	"PYTHON":          true,
	"PYTHONPATH":      true,
	"AR":              true,
	"NM":              true,
	"RANLIB":          true,
	"PKG_CONFIG_PATH": true,
	"RUSTUP_HOME":     true,
	"CARGO_HOME":      true,
	"RUSTFLAGS":       true,

	// Safe Node & package manager flags
	"NODE":                          true,
	"NODE_ENV":                      true,
	"NODE_PATH":                     true,
	"CI":                            true,
	"TMPDIR":                        true,
	"TEMP":                          true,
	"TMP":                           true,
	"XFPM":                          true,
	"npm_config_user_agent":         true,
	"npm_config_foreground_scripts": true,
	"npm_lifecycle_event":           true,
	"npm_lifecycle_script":          true,
	"npm_node_execpath":             true,
	"npm_execpath":                  true,
	"npm_package_name":              true,
	"npm_package_version":           true,
}

func isLifecycleAllowedEnv(key string) bool {
	if lifecycleAllowedEnv[key] || lifecycleAllowedEnv[strings.ToUpper(key)] {
		return true
	}
	if strings.HasPrefix(key, "npm_config_") || strings.HasPrefix(key, "npm_package_") || strings.HasPrefix(key, "LC_") {
		return true
	}
	return false
}

func findXessRunner() string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "xess-runner")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".xfpm", "bin", "xess-runner")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath("xess-runner"); err == nil {
		return path
	}
	return ""
}

type ScriptRunner struct {
	maxParallel    int
	timeout        time.Duration
	projectRoot    string
	xessRunnerPath string
	canaryPath     string
	targetInodes   string
	envKeysToPurge map[string]bool
}

func NewScriptRunner(projectRoot string) *ScriptRunner {
	maxParallel := runtime.NumCPU()
	if maxParallel < 4 {
		maxParallel = 4
	}
	return &ScriptRunner{
		maxParallel:    maxParallel,
		timeout:        5 * time.Minute,
		projectRoot:    projectRoot,
		xessRunnerPath: findXessRunner(),
		envKeysToPurge: make(map[string]bool),
	}
}

func (r *ScriptRunner) setupCanarySandbox() {
	commonEnvNames := []string{
		".env",
		".env.local",
		".env.development",
		".env.production",
		".env.staging",
		".env.test",
	}

	var inodes []string
	discoveredKeys := make(map[string]bool)

	for _, name := range commonEnvNames {
		envPath := filepath.Join(r.projectRoot, name)
		if fi, err := os.Stat(envPath); err == nil && !fi.IsDir() {
			if inode := getSyscallInode(fi); inode != "" {
				inodes = append(inodes, inode)
			}
			if data, err := os.ReadFile(envPath); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					if idx := strings.Index(line, "="); idx != -1 {
						k := strings.TrimSpace(line[:idx])
						if k != "" {
							discoveredKeys[k] = true
							if r.envKeysToPurge == nil {
								r.envKeysToPurge = make(map[string]bool)
							}
							r.envKeysToPurge[k] = true
						}
					}
				}
			}
		}
	}

	if len(inodes) == 0 || r.xessRunnerPath == "" || runtime.GOOS != "linux" {
		return
	}

	r.targetInodes = strings.Join(inodes, ",")

	tmpDir := paths.LocalTmpDir(r.projectRoot)
	_ = utils.CreateDirAllSecure(tmpDir)
	canaryPath := filepath.Join(tmpDir, fmt.Sprintf(".xfpm_lifecycle_canary_%d.env", os.Getpid()))

	var sb strings.Builder
	sb.WriteString("# ====================================================================\n")
	sb.WriteString("# XYPRISS ENVIRONMENT SECURITY SHIELD - CANARY HONEYPOT (LIFECYCLE HOOKS)\n")
	sb.WriteString("# Supply-Chain Attack Neutralization (Ref: Issue #52)\n")
	sb.WriteString("# ====================================================================\n")

	randToken := fmt.Sprintf("%x", time.Now().UnixNano())
	if len(randToken) > 8 {
		randToken = randToken[:8]
	}

	for k := range discoveredKeys {
		lowerK := strings.ToLower(strings.ReplaceAll(k, "_", "-"))
		sb.WriteString(fmt.Sprintf("%s=xy_canary_tarpit_%s_%s\n", k, lowerK, randToken))
	}

	if err := os.WriteFile(canaryPath, []byte(sb.String()), 0600); err == nil {
		r.canaryPath = canaryPath
	}
}

func (r *ScriptRunner) cleanup() {
	if r.canaryPath != "" {
		_ = os.Remove(r.canaryPath)
		r.canaryPath = ""
	}
}

func (r *ScriptRunner) ExecuteParallel(ctx context.Context, tasks []ScriptTask) error {
	if len(tasks) == 0 {
		return nil
	}

	r.setupCanarySandbox()
	defer r.cleanup()

	if !utils.SilentMode {
		fmt.Printf("\n  [SCRIPTS] Executing %d tasks...\n", len(tasks))
		if r.xessRunnerPath != "" && r.canaryPath != "" {
			utils.Success("libXESS loaded")
		}
	}

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

	var cmd *exec.Cmd
	if r.xessRunnerPath != "" && r.canaryPath != "" && runtime.GOOS == "linux" {
		cmd = exec.CommandContext(ctx, r.xessRunnerPath, "sh", "-c", task.ScriptCommand)
	} else {
		cmd = utils.GetShellCommand(ctx, task.ScriptCommand)
	}
	cmd.Dir = task.PackageDir
	cmd.Env = r.buildEnv(pathVal)

	if r.xessRunnerPath != "" && r.canaryPath != "" {
		cmd.Env = append(cmd.Env,
			"XESS_CANARY_PATH="+r.canaryPath,
			"XESS_GUARD_MODE=honeypot",
		)
		if r.targetInodes != "" {
			cmd.Env = append(cmd.Env, "XESS_TARGET_INODES="+r.targetInodes)
		}
	}

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
	envPaths := []string{}

	// 1. Package's .bin
	packageBin := filepath.Join(filepath.Dir(packageDir), ".bin")
	if _, err := os.Stat(packageBin); err == nil {
		envPaths = append(envPaths, packageBin)
	}

	// 2. Project's standard .bin
	projectBin := paths.LocalBinDir(r.projectRoot)
	if _, err := os.Stat(projectBin); err == nil {
		envPaths = append(envPaths, projectBin)
	}

	// 3. XFPM's internal .bin (fallback/legacy)
	internalBin := filepath.Join(paths.LocalXpmDir(r.projectRoot), ".bin")
	if _, err := os.Stat(internalBin); err == nil {
		envPaths = append(envPaths, internalBin)
	}

	// 4. Global XPM bin
	globalBin := paths.BinDir()
	if _, err := os.Stat(globalBin); err == nil {
		envPaths = append(envPaths, globalBin)
	}

	// 4. System PATH
	envPaths = append(envPaths, os.Getenv("PATH"))

	return strings.Join(envPaths, string(os.PathListSeparator))
}

func (r *ScriptRunner) buildEnv(pathVal string) []string {
	var sanitized []string
	for _, envStr := range os.Environ() {
		parts := strings.SplitN(envStr, "=", 2)
		if len(parts) == 2 {
			k := parts[0]
			// Strict purge: if the key is explicitly found in .env files, purge it
			if r.envKeysToPurge != nil && r.envKeysToPurge[k] {
				continue
			}
			// Strict Zero-Trust whitelist: only allow safe system/toolchain variables
			if isLifecycleAllowedEnv(k) {
				sanitized = append(sanitized, envStr)
			}
		}
	}

	// Keep existing vars but override key ones
	overrides := map[string]string{
		"PATH":                          pathVal,
		"NODE_ENV":                      "production",
		"CI":                            "true",
		"npm_config_foreground_scripts": "true",
		"TMPDIR":                        paths.LocalTmpDir(r.projectRoot),
		"TEMP":                          paths.LocalTmpDir(r.projectRoot),
		"TMP":                           paths.LocalTmpDir(r.projectRoot),
		"XFPM":                          "true",
		"npm_config_user_agent":         "xfpm",
	}

	// 1. NODE_PATH addition
	nodePath := filepath.Join(r.projectRoot, "node_modules")
	if existing := os.Getenv("NODE_PATH"); existing != "" {
		nodePath = nodePath + string(os.PathListSeparator) + existing
	}
	overrides["NODE_PATH"] = nodePath

	// Ensure the temp dir exists
	utils.CreateDirAllSecure(overrides["TMPDIR"])

	newEnv := utils.CleanEnvForOverride(sanitized, overrides)

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
	if utils.SilentMode {
		// Drain the pipe pour eviter que le process fils se bloque sur un write
		io.Copy(io.Discard, rdr)
		return
	}
	scanner := bufio.NewScanner(rdr)
	for scanner.Scan() {
		fmt.Printf("   %s %s\n", utils.DimColor.Sprint(prefix), scanner.Text())
	}
}
