package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/xfpm-go/internal/utils"
	"github.com/pterm/pterm"
)

type Installer struct {
	cas             *Cas
	registry        *RegistryClient
	projectRoot     string
	vstoreRoot      string
	bar             *pterm.ProgressbarPrinter
	Force           bool
	ForcePackages   map[string]bool   // Specific packages to force extraction (ignore cache)
	IsGlobal        bool
	DirectDeps      map[string]string // name -> version req from root package.json
	changedPackages sync.Map          // tracks packages actually downloaded this session
}

func NewInstaller(cas *Cas, registry *RegistryClient, projectRoot string) *Installer {
	// virtual store always lives under projectRoot/node_modules/.xpm/virtual_store
	// (matches Rust's get_virtual_store_root which joins .xpm_global/node_modules/.xpm/virtual_store)
	vstoreRoot := filepath.Join(projectRoot, "node_modules", ".xpm", "virtual_store")
	os.MkdirAll(vstoreRoot, 0755)
	return &Installer{
		cas:         cas,
		registry:    registry,
		projectRoot: projectRoot,
		vstoreRoot:  vstoreRoot,
	}
}

func (i *Installer) Install(ctx context.Context, packages []*ResolvedPackage) error {
	utils.Info("Starting installation of %d packages...", len(packages))
	
	
	i.bar, _ = pterm.DefaultProgressbar.
		WithTotal(len(packages)).
		WithTitle(utils.MatrixColor.Sprint("  [INIT_SEQ]")).
		WithBarCharacter("█").
		WithLastCharacter("█").
		WithRemoveWhenDone(true).
		Start()



	// 1. Batch extraction
	// Ensure we only process unique Name@Version pairs to avoid race conditions during extraction/linking
	uniquePackages := make([]*ResolvedPackage, 0, len(packages))
	seen := make(map[string]bool)
	for _, p := range packages {
		key := p.Name + "@" + p.Version
		if !seen[key] {
			seen[key] = true
			uniquePackages = append(uniquePackages, p)
		}
	}
	packages = uniquePackages

	if err := i.batchEnsureExtracted(ctx, packages); err != nil {
		i.bar.Stop()
		return err
	}

	// 2. Link dependencies in virtual store
	if err := i.linkPackageDeps(packages); err != nil {
		return err
	}

	// 3. Link binaries
	if err := i.batchLinkBinaries(packages); err != nil {
		return err
	}

	// 4. Link to root node_modules
	if err := i.linkToRoot(packages); err != nil {
		return err
	}

	i.bar.Stop()
	changedCount := 0
	i.changedPackages.Range(func(_, _ interface{}) bool {
		changedCount++
		return true
	})

	skippedCount := len(packages) - changedCount

	if changedCount == 0 {
		utils.Success("All %d dependencies are already up to date.", len(packages))
	} else {
		utils.Success("Installation complete: %d updated, %d unchanged.", changedCount, skippedCount)
	}

	// 5. Run lifecycle scripts
	if err := i.runLifecycleScripts(ctx, packages); err != nil {
		return err
	}

	// 6. Global binary export (mirrors Rust Phase 6)
	if i.IsGlobal {
		binDir := filepath.Join(i.projectRoot, "bin")
		os.MkdirAll(binDir, 0755)
		vstoreName := ""
		for _, pkg := range packages {
			// Only export direct deps
			if _, isDirect := i.DirectDeps[pkg.Name]; !isDirect {
				continue
			}
			if pkg.Metadata.Bin == nil {
				continue
			}
			vstoreName = strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
			pkgDir := filepath.Join(i.vstoreRoot, vstoreName, "node_modules", pkg.Name)
			i.linkBinaries(pkgDir, binDir, pkg.Metadata.Bin)
		}
		utils.Info("Global binaries exported to: %s", binDir)
		// Ensure PATH is configured
		ensureGlobalPath(binDir)
	}

	return nil
}

func (i *Installer) batchEnsureExtracted(ctx context.Context, packages []*ResolvedPackage) error {
	concurrency := 16 
	sem := make(chan struct{}, concurrency)
	errChan := make(chan error, len(packages))
	var wg sync.WaitGroup

	var mu sync.Mutex
	lastPkg := ""

	// Dynamic Matrix Loop
	stopMatrix := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				pkg := lastPkg
				mu.Unlock()
				if pkg != "" {
					m := utils.RandomMatrixString(5)
					i.bar.UpdateTitle(fmt.Sprintf("  %s %s %s  ", 
						pterm.FgCyan.Sprint("[EXTRACTING]"), 
						utils.MatrixColor.Sprint(m), 
						utils.DimColor.Sprint(pkg),
					))
				}
			case <-stopMatrix:
				return
			}
		}
	}()

	for _, pkg := range packages {
		wg.Add(1)
		go func(p *ResolvedPackage) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				
				mu.Lock()
				lastPkg = p.Name
				mu.Unlock()

				if err := i.ensureExtracted(ctx, p); err != nil {
					errChan <- err
				}
				mu.Lock()
				i.bar.Increment()
				mu.Unlock()
			case <-ctx.Done():
				return
			}
		}(pkg)
	}

	wg.Wait()
	close(stopMatrix)
	close(errChan)

	if len(errChan) > 0 {
		return <-errChan
	}
	
	i.bar.Current = i.bar.Total
	i.bar.UpdateTitle("  " + pterm.BgBlue.Sprint(" SUCCESS ") + "  " + utils.GreenColor.Sprint("Installation phase complete.") + "  ")
	fmt.Println()

	return nil
}

func (i *Installer) ensureExtracted(ctx context.Context, pkg *ResolvedPackage) error {
	pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
	pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)

	// Determine if we should force extraction for this specific package
	shouldForce := i.Force
	if !shouldForce && i.ForcePackages != nil {
		if i.ForcePackages[pkg.Name] {
			shouldForce = true
		}
	}

	if !shouldForce {
		if _, err := os.Stat(filepath.Join(pkgDir, "package.json")); err == nil {
			return nil
		}
	} else {
		// Only remove if it was not already re-extracted in this session
		cacheKey := pkg.Name + "@" + pkg.Version
		if _, exists := i.changedPackages.Load(cacheKey); !exists {
			// Surgically remove only the package content directory, not the parent
			os.RemoveAll(pkgDir)
		}
	}

	pterm.Printf("   %s %-30s %s\n", utils.GreenColor.Sprint("├─"), pkg.Name+"@"+pkg.Version, utils.DimColor.Sprint("extracting..."))

	// Download and extract
	stream, err := i.registry.DownloadTarballStream(ctx, pkg.Metadata.Dist.Tarball)
	if err != nil {
		return err
	}
	defer stream.Close()

	extractor := NewStreamingExtractor(i.cas)
	fileMap, err := extractor.Extract(stream)
	if err != nil {
		return err
	}

	// Store index in CAS
	if err := i.cas.StoreIndex(pkg.Name, pkg.Version, fileMap); err != nil {
		return err
	}

	if err := i.LinkFilesToDir(pkgDir, fileMap); err != nil {
		return err
	}

	// Mark as changed — scripts will run for this package
	cacheKey := pkg.Name + "@" + pkg.Version
	i.changedPackages.Store(cacheKey, true)
	return nil
}

func (i *Installer) LinkFilesToDir(destDir string, index map[string]string) error {
	os.MkdirAll(destDir, 0755)

	for relPath, hash := range index {
		normalized := relPath
		if idx := strings.Index(relPath, "/"); idx != -1 {
			normalized = relPath[idx+1:]
		}

		destPath := filepath.Join(destDir, normalized)
		sourcePath := i.cas.GetFilePath(hash)

		os.MkdirAll(filepath.Dir(destPath), 0755)

		// ALWAYS remove the destination before linking or copying.
		// If we don't, and destPath is a hardlink to sourcePath, os.Create(destPath)
		// will truncate the inode and thus empty the source file before we read it!
		os.Remove(destPath)

		if err := os.Link(sourcePath, destPath); err != nil {
			// If link fails (cross-device or already exists), fallback to copy
			// Log the reason for fallback
			utils.Log("DEBUG", fmt.Sprintf("Failed to link %s to %s: %v. Falling back to copy.", sourcePath, destPath, err))
			if err := i.copyFile(sourcePath, destPath); err != nil {
				return err
			}
			// Verify copied file size
			fi_src, _ := os.Stat(sourcePath)
			fi_dst, _ := os.Stat(destPath)
			if fi_src != nil && fi_dst != nil {
				if fi_dst.Size() != fi_src.Size() {
					utils.Error("[LINK] CRITICAL: Size mismatch after copy! Source: %d, Dest: %d. File: %s", fi_src.Size(), fi_dst.Size(), destPath)
				}
			}
		}
	}
	return nil
}

func (i *Installer) linkPackageDeps(packages []*ResolvedPackage) error {
	for _, pkg := range packages {
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDepsNM := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules")

		for depName, depVersion := range pkg.ResolvedDependencies {
			targetLink := filepath.Join(pkgDepsNM, depName)
			os.MkdirAll(filepath.Dir(targetLink), 0755)

			depVStoreName := strings.ReplaceAll(depName, "/", "+") + "@" + depVersion
			depAbsTarget := filepath.Join(i.vstoreRoot, depVStoreName, "node_modules", depName)

			os.Remove(targetLink)
			if err := os.Symlink(depAbsTarget, targetLink); err != nil {
				// Continue 
			}
		}
	}
	return nil
}

func (i *Installer) batchLinkBinaries(packages []*ResolvedPackage) error {
	for _, pkg := range packages {
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgAbsPath := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		targetBinDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", ".bin")

		if err := i.linkBinaries(pkgAbsPath, targetBinDir, pkg.Metadata.Bin); err != nil {
			// Handle bin linking
		}
	}
	return nil
}

func (i *Installer) linkBinaries(pkgDir, binDir string, bin json.RawMessage) error {
	if bin == nil {
		return nil
	}

	os.MkdirAll(binDir, 0755)

	type BinObject map[string]string
	var binMap BinObject
	var binStr string

	if err := json.Unmarshal(bin, &binMap); err == nil {
		for name, relPath := range binMap {
			i.createBinLink(name, pkgDir, relPath, binDir)
		}
	} else if err := json.Unmarshal(bin, &binStr); err == nil {
		name := filepath.Base(pkgDir)
		i.createBinLink(name, pkgDir, binStr, binDir)
	}

	return nil
}

func (i *Installer) createBinLink(name, pkgDir, relPath, binDir string) {
	dest := filepath.Join(binDir, name)
	os.Remove(dest)

	absTarget := filepath.Join(pkgDir, relPath)
	relTarget, err := filepath.Rel(binDir, absTarget)
	if err != nil {
		relTarget = absTarget
	}

	if runtime.GOOS != "windows" {
		os.Symlink(relTarget, dest)
		os.Chmod(absTarget, 0755)
	} else {
		// Windows symlink or shim (simplified)
		os.Symlink(relTarget, dest)
	}
}

func (i *Installer) linkToRoot(packages []*ResolvedPackage) error {
	rootBinDir := filepath.Join(i.projectRoot, "node_modules", ".bin")
	os.MkdirAll(rootBinDir, 0755)

	// Build a set of direct dependency names for priority ordering
	directNames := make(map[string]bool)
	for name := range i.DirectDeps {
		directNames[name] = true
	}

	linkPkg := func(pkg *ResolvedPackage) {
		rootNM := filepath.Join(i.projectRoot, "node_modules", pkg.Name)
		os.MkdirAll(filepath.Dir(rootNM), 0755)

		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		absTarget := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)

		os.Remove(rootNM)
		os.Symlink(absTarget, rootNM)

		if pkg.Metadata.Bin != nil {
			i.linkBinaries(absTarget, rootBinDir, pkg.Metadata.Bin)
		}
	}

	// Pass 1: Link transitive dependencies (lower priority)
	for _, pkg := range packages {
		if !directNames[pkg.Name] {
			linkPkg(pkg)
		}
	}
	// Pass 2: Link direct dependencies last (highest priority — they overwrite transitives)
	for _, pkg := range packages {
		if version, ok := i.DirectDeps[pkg.Name]; ok {
			if pkg.Version == version {
				linkPkg(pkg)
			}
		}
	}
	return nil
}

func (i *Installer) runLifecycleScripts(ctx context.Context, packages []*ResolvedPackage) error {
	runner := NewScriptRunner(i.projectRoot)
	tasks := []ScriptTask{}

	lifecycleOrder := []string{"preinstall", "install", "postinstall"}

	for _, pkg := range packages {
		// Only run scripts for packages actually downloaded this session (mirrors Rust changed_packages)
		cacheKey := pkg.Name + "@" + pkg.Version
		if _, wasChanged := i.changedPackages.Load(cacheKey); !wasChanged {
			continue
		}

		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)

		pkgJson, err := LoadPackageJson(filepath.Join(pkgDir, "package.json"))
		if err != nil {
			continue
		}

		for _, scriptType := range lifecycleOrder {
			if cmd, ok := pkgJson.Scripts[scriptType]; ok {
				tasks = append(tasks, ScriptTask{
					PackageName:    pkg.Name,
					PackageVersion: pkg.Version,
					PackageDir:     pkgDir,
					ScriptType:     scriptType,
					ScriptCommand:  cmd,
				})
			}
		}
	}

	return runner.ExecuteParallel(ctx, tasks)
}

func (i *Installer) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		// If we can't remove it, it might still fail later, but we try
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// ensureGlobalPath ensures the global bin dir is in PATH by appending to shell rc files.
func ensureGlobalPath(binDir string) {
	exportLine := fmt.Sprintf("\nexport PATH=\"%s:$PATH\"", binDir)
	home, _ := os.UserHomeDir()
	rcFiles := []string{
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".profile"),
	}
	for _, rc := range rcFiles {
		data, err := os.ReadFile(rc)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), binDir) {
			continue // already configured
		}
		f, err := os.OpenFile(rc, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}
		f.WriteString(exportLine)
		f.Close()
	}
}
