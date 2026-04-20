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

	"github.com/Nehonix-Team/XFMP/internal/utils"
	"github.com/pterm/pterm"
)

type Installer struct {
	cas             *Cas
	registry        *RegistryClient
	projectRoot     string
	vstoreRoot      string // node_modules/.xpm/vstore
	bar             *pterm.ProgressbarPrinter
	Force           bool
	ForcePackages   map[string]bool
	IsGlobal        bool
	DirectDeps      map[string]string
	changedPackages sync.Map
	linkingPool     *utils.AdaptiveSemaphore
	linkedPaths     sync.Map // string -> bool (to avoid race collisions)
	barMu           sync.Mutex
	isBarPaused     bool
}

func NewInstaller(cas *Cas, registry *RegistryClient, projectRoot string) *Installer {
	// Local Virtual Store for Ancestor Hoisting
	vstoreRoot := filepath.Join(projectRoot, "node_modules", ".xpm", "vstore")
	utils.CreateDirAllSecure(vstoreRoot)

	return &Installer{
		cas:         cas,
		registry:    registry,
		projectRoot: projectRoot,
		vstoreRoot:  vstoreRoot,
		linkingPool: utils.NewAdaptiveSemaphore(128),
	}
}

func (i *Installer) Install(ctx context.Context, packages []*ResolvedPackage) error {
	utils.Info("Starting installation of %d packages via Ancestor Hoisting...", len(packages))

	i.bar, _ = pterm.DefaultProgressbar.
		WithTotal(len(packages)).
		WithTitle(utils.MatrixColor.Sprint("  [INIT_SEQ]")).
		WithBarCharacter("█").
		WithLastCharacter("█").
		WithRemoveWhenDone(true).
		Start()

	uniquePackages := i.getUniquePackages(packages)

	if err := i.batchEnsureExtracted(ctx, uniquePackages, i.vstoreRoot); err != nil {
		i.bar.Stop()
		return err
	}

	if err := i.linkPackageDeps(uniquePackages, i.vstoreRoot); err != nil {
		i.bar.Stop()
		return err
	}

	if err := i.linkToRoot(uniquePackages); err != nil {
		i.bar.Stop()
		return err
	}

	i.bar.Stop()
	
	changedCount := 0
	i.changedPackages.Range(func(_, _ interface{}) bool {
		changedCount++
		return true
	})

	if changedCount == 0 {
		utils.Success("All dependencies are up to date.")
	} else {
		utils.Success("Installation complete: %d updated.", changedCount)
	}

	// ALWAYS run lifecycle scripts BEFORE exporting global binaries
	// This ensures that tools like 'bun' which download their binaries during postinstall
	// have the binaries ready for linking.
	if err := i.runLifecycleScripts(ctx, packages); err != nil {
		return err
	}

	if i.IsGlobal {
		i.exportGlobalBinaries(packages)
		utils.EnsurePathInShell()
	}

	return nil
}

func (i *Installer) getUniquePackages(packages []*ResolvedPackage) []*ResolvedPackage {
	unique := make([]*ResolvedPackage, 0, len(packages))
	seen := make(map[string]bool)
	for _, p := range packages {
		key := p.Name + "@" + p.Version
		if !seen[key] {
			seen[key] = true
			unique = append(unique, p)
		}
	}
	return unique
}

func (i *Installer) batchEnsureExtracted(ctx context.Context, packages []*ResolvedPackage, targetVStore string) error {
	if len(packages) == 0 { return nil }
	concurrency := 16
	sem := make(chan struct{}, concurrency)
	errChan := make(chan error, len(packages))
	var wg sync.WaitGroup
	var mu sync.Mutex
	lastPkg := ""

	startTime := time.Now()
	stopProgress := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				i.barMu.Lock()
				paused := i.isBarPaused
				i.barMu.Unlock()
				if paused { continue }

				mu.Lock()
				pkg := lastPkg
				mu.Unlock()
				if pkg != "" {
					elapsed := time.Since(startTime).Truncate(time.Millisecond)
					i.bar.UpdateTitle(fmt.Sprintf("  %s %s %s  ", pterm.FgCyan.Sprint("[EXTRACTING]"), utils.AccentColor.Sprint(elapsed.String()), utils.DimColor.Sprint(pkg)))
				}
			case <-stopProgress: return
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
				if err := i.ensureExtracted(ctx, p, targetVStore); err != nil { errChan <- err }
				i.barMu.Lock()
				if !i.isBarPaused {
					i.bar.Increment()
				}
				i.barMu.Unlock()
			case <-ctx.Done(): return
			}
		}(pkg)
	}
	wg.Wait()
	close(stopProgress)
	close(errChan)
	if len(errChan) > 0 { return <-errChan }
	return nil
}

func (i *Installer) ensureExtracted(ctx context.Context, pkg *ResolvedPackage, targetVStore string) error {
	pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
	pkgDir := filepath.Join(targetVStore, pkgVStoreName, "node_modules", pkg.Name)

	if !i.Force {
		if _, err := os.Stat(filepath.Join(pkgDir, "package.json")); err == nil { return nil }
	}

	if pkg.LocalPath != "" { return i.extractLocal(pkg, targetVStore) }

	pterm.Printf("   %s %-30s %s\n", utils.GreenColor.Sprint("├─"), pkg.Name+"@"+pkg.Version, utils.DimColor.Sprint("reflinking..."))
	
	stream, err := i.registry.DownloadTarballStream(ctx, pkg.Metadata.Dist.Tarball)
	if err != nil { return err }
	defer stream.Close()

	extractor := NewStreamingExtractor(i.cas)
	fileMap, err := extractor.Extract(stream)
	if err != nil { return err }

	i.cas.StoreIndex(pkg.Name, pkg.Version, fileMap)
	i.LinkFilesToDir(pkgDir, fileMap)
	i.changedPackages.Store(pkg.Name+"@"+pkg.Version, true)

	if sig, err := os.Stat(filepath.Join(pkgDir, "xypriss.plugin.sig")); err == nil && !sig.IsDir() {
		if err := i.verifySignatureTrust(filepath.Join(pkgDir, "xypriss.plugin.sig"), pkg); err != nil {
			return err
		}
	}

	if pkg.IsRevoked {
		i.patchPackageJsonForRevocation(pkgDir, pkg)
	}

	return nil
}

func (i *Installer) LinkFilesToDir(destDir string, index map[string]string) error {
	utils.CreateDirAllSecure(destDir)
	
	// Speed boost: Parallelize file linking within the package
	// We use a small local pool to avoid overwhelming the OS with open files
	concurrency := 8
	if len(index) < concurrency { concurrency = 1 }
	
	type linkJob struct { relPath, hash string }
	jobs := make(chan linkJob, len(index))
	for r, h := range index { jobs <- linkJob{r, h} }
	close(jobs)

	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				normalized := j.relPath
				if idx := strings.Index(j.relPath, "/"); idx != -1 { 
					normalized = j.relPath[idx+1:] 
				}
				destPath := filepath.Join(destDir, normalized)
				sourcePath := i.cas.GetFilePath(j.hash)
				
				utils.CreateDirAllSecure(filepath.Dir(destPath))
				os.Remove(destPath)
				
				err := utils.Reflink(sourcePath, destPath)
				if err == nil {
					i.applyPermissions(sourcePath, destPath)
					continue
				}

				if err := os.Link(sourcePath, destPath); err == nil {
					i.applyPermissions(sourcePath, destPath)
					continue
				}

				if err := i.copyFile(sourcePath, destPath); err == nil {
					i.applyPermissions(sourcePath, destPath)
				}
			}
		}()
	}
	wg.Wait()
	return nil
}

func (i *Installer) applyPermissions(src, dst string) {
	if runtime.GOOS == "windows" { return }
	if fi, err := os.Stat(src); err == nil {
		if (fi.Mode() & 0111) != 0 {
			os.Chmod(dst, 0755)
		} else {
			os.Chmod(dst, 0644)
		}
	}
}

func (i *Installer) linkPackageDeps(packages []*ResolvedPackage, vstoreBase string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(packages))

	for _, pkg := range packages {
		wg.Add(1)
		go func(p *ResolvedPackage) {
			defer wg.Done()
			
			// Use global linking pool
			if err := i.linkingPool.Acquire(context.Background(), 128); err != nil {
				return
			}
			defer i.linkingPool.Release()

			pkgVStoreName := strings.ReplaceAll(p.Name, "/", "+") + "@" + p.Version
			pkgDepsNM := filepath.Join(vstoreBase, pkgVStoreName, "node_modules")

			for depName, depVersion := range p.ResolvedDependencies {
				targetLink := filepath.Join(pkgDepsNM, depName)
				
				// COLLISION GUARD: Though each unique version has its own vstore NM,
				// if we process uniquePackages, we only ever visit each once.
				// However, redundancy might exist if resolvedDependencies overlaps.
				if _, seen := i.linkedPaths.LoadOrStore(targetLink, true); seen {
					continue
				}

				utils.CreateDirAllSecure(filepath.Dir(targetLink))
				depVStoreName := strings.ReplaceAll(depName, "/", "+") + "@" + depVersion
				steps := strings.Count(depName, "/") + 2
				relPrefix := ""
				for j := 0; j < steps; j++ {
					relPrefix += "../"
				}
				relTarget := filepath.Join(relPrefix, depVStoreName, "node_modules", depName)

				os.Remove(targetLink)
				if err := utils.LinkDir(relTarget, targetLink); err != nil {
					utils.Error("Failed to link dependency %s -> %s: %v", depName, targetLink, err)
				}
			}
		}(pkg)
	}

	wg.Wait()
	close(errChan)
	if len(errChan) > 0 {
		return <-errChan
	}
	return nil
}

func (i *Installer) linkToRoot(packages []*ResolvedPackage) error {
	rootNMDir := filepath.Join(i.projectRoot, "node_modules")
	rootBinDir := filepath.Join(rootNMDir, ".bin")
	utils.CreateDirAllSecure(rootBinDir)

	var wg sync.WaitGroup
	linkPkg := func(pkg *ResolvedPackage) {
		defer wg.Done()

		rootNM := filepath.Join(rootNMDir, pkg.Name)
		// COLLISION GUARD: First worker to reach a path wins.
		// For root packages, we prioritize those in i.DirectDeps below.
		if _, seen := i.linkedPaths.LoadOrStore(rootNM, true); seen {
			return
		}
		
		if err := i.linkingPool.Acquire(context.Background(), 128); err != nil {
			return
		}
		defer i.linkingPool.Release()

		utils.CreateDirAllSecure(filepath.Dir(rootNM))
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version

		absTarget := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		relTarget, _ := filepath.Rel(filepath.Dir(rootNM), absTarget)

		os.Remove(rootNM)
		if err := utils.LinkDir(relTarget, rootNM); err != nil {
			utils.Error("Failed to link root package %s -> %s: %v", pkg.Name, rootNM, err)
		}

		if pkg.Metadata.Bin != nil {
			i.linkBinaries(absTarget, rootBinDir, pkg.Metadata.Bin)
		}
	}

	// PHASE 1: Direct Dependencies (Root packages WIN)
	for _, pkg := range packages {
		if v, ok := i.DirectDeps[pkg.Name]; ok && pkg.Version == v {
			wg.Add(1)
			go linkPkg(pkg)
		}
	}
	wg.Wait() // Ensure root packages are linked first

	// PHASE 2: Indirect Dependencies (Transitive packages)
	directNames := make(map[string]bool)
	for name := range i.DirectDeps {
		directNames[name] = true
	}

	for _, pkg := range packages {
		if !directNames[pkg.Name] {
			wg.Add(1)
			go linkPkg(pkg)
		}
	}

	wg.Wait()
	return nil
}

func (i *Installer) runLifecycleScripts(ctx context.Context, packages []*ResolvedPackage) error {
	runner := NewScriptRunner(i.projectRoot)
	tasks := []ScriptTask{}
	lifecycleOrder := []string{"preinstall", "install", "postinstall"}
	
	for _, pkg := range packages {
		cacheKey := pkg.Name + "@" + pkg.Version
		if _, wasChanged := i.changedPackages.Load(cacheKey); !wasChanged { continue }
		
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		
		pkgJson, err := LoadPackageJson(filepath.Join(pkgDir, "package.json"))
		if err != nil { continue }
		
		for _, scriptType := range lifecycleOrder {
			if cmd, ok := pkgJson.Scripts[scriptType]; ok {
				tasks = append(tasks, ScriptTask{
					PackageName: pkg.Name, PackageVersion: pkg.Version,
					PackageDir: pkgDir, ScriptType: scriptType, ScriptCommand: cmd,
				})
			}
		}
	}
	return runner.ExecuteParallel(ctx, tasks)
}

func (i *Installer) linkBinaries(pkgDir, binDir string, bin json.RawMessage) error {
	if bin == nil { return nil }
	utils.CreateDirAllSecure(binDir)
	type BinObject map[string]string
	var binMap BinObject
	var binStr string
	if err := json.Unmarshal(bin, &binMap); err == nil {
		for name, relPath := range binMap { i.adaptiveBinLink(name, pkgDir, relPath, binDir) }
	} else if err := json.Unmarshal(bin, &binStr); err == nil {
		name := filepath.Base(pkgDir)
		i.adaptiveBinLink(name, pkgDir, binStr, binDir)
	}
	return nil
}

func (i *Installer) adaptiveBinLink(name, pkgDir, relPath, binDir string) {
	dest := filepath.Join(binDir, name)
	// COLLISION GUARD: First binary to claim the name wins
	if _, seen := i.linkedPaths.LoadOrStore(dest, true); seen {
		return
	}
	os.Remove(dest)

	// CLEAN RELATIVE PATH (Handle .exe suffixes in Unix or nested bins)
	cleanRel := strings.TrimSuffix(relPath, ".exe")
	if runtime.GOOS == "windows" && !strings.HasSuffix(cleanRel, ".exe") {
		cleanRel += ".exe"
	}

	absTarget := filepath.Join(pkgDir, cleanRel)

	// ADAPTIVE DISCOVERY (If metadata says bin/bun.exe but file is bin/bun)
	if _, err := os.Stat(absTarget); os.IsNotExist(err) {
		// Try literally as specified in metadata
		absTarget = filepath.Join(pkgDir, relPath)
		if _, err := os.Stat(absTarget); os.IsNotExist(err) {
			// Search for any executable with the same base name in common bin folders
			bases := []string{"bin", "dist", "."}
			for _, b := range bases {
				target := filepath.Join(pkgDir, b, name)
				if _, err := os.Stat(target); err == nil {
					absTarget = target
					break
				}
			}
		}
	}

	relTarget, _ := filepath.Rel(binDir, absTarget)
	if runtime.GOOS != "windows" {
		os.Symlink(relTarget, dest)
		// Ensure it's executable
		os.Chmod(absTarget, 0755)
		i.ensureExecutableRecursive(filepath.Dir(absTarget))
	} else {
		if err := utils.LinkDir(relTarget, dest); err != nil {
			utils.Error("Failed to link binary %s -> %s: %v", name, dest, err)
		}

		// GENERATE SHIMS FOR JS BINARIES ON WINDOWS
		// This allows running 'eslint' instead of 'node node_modules/eslint/bin/eslint'
		// It only applies if the target is NOT an .exe
		if !strings.HasSuffix(strings.ToLower(absTarget), ".exe") {
			utils.GenerateBinShim(binDir, name, relTarget)
		}
	}
}

func (i *Installer) ensureExecutableRecursive(path string) {
	if _, err := os.Stat(path); err != nil { return }
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return nil }
		os.Chmod(p, 0755)
		return nil
	})
}

func (i *Installer) exportGlobalBinaries(packages []*ResolvedPackage) {
	home, _ := os.UserHomeDir()
	globalBinDir := filepath.Join(home, ".xpm", "bin")
	utils.CreateDirAllSecure(globalBinDir)

	for _, pkg := range packages {
		if _, isDirect := i.DirectDeps[pkg.Name]; !isDirect { continue }
		if pkg.Metadata.Bin == nil { continue }
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		i.linkBinaries(pkgDir, globalBinDir, pkg.Metadata.Bin)
	}
}

func (i *Installer) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()
	os.Remove(dst)
	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (i *Installer) extractLocal(pkg *ResolvedPackage, targetVStore string) error {
	pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
	pkgDir := filepath.Join(targetVStore, pkgVStoreName, "node_modules", pkg.Name)
	os.RemoveAll(pkgDir)
	fileMap := make(map[string]string)
	localPkg, _ := LoadPackageJson(filepath.Join(pkg.LocalPath, "package.json"))
	hasFiles := localPkg != nil && len(localPkg.Files) > 0

	filepath.Walk(pkg.LocalPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return nil }
		relPath, _ := filepath.Rel(pkg.LocalPath, path)
		if hasFiles {
			included := false
			for _, pat := range localPkg.Files {
				if strings.HasPrefix(relPath, filepath.Clean(pat)) { included = true; break }
			}
			if !included && relPath != "package.json" { return nil }
		}
		file, _ := os.Open(path)
		defer file.Close()
		hash, _ := i.cas.StoreStream(file, (info.Mode()&0111) != 0)
		fileMap["package/"+relPath] = hash
		return nil
	})
	i.cas.StoreIndex(pkg.Name, pkg.Version, fileMap)
	i.LinkFilesToDir(pkgDir, fileMap)
	i.changedPackages.Store(pkg.Name+"@"+pkg.Version, true)

	if sig, err := os.Stat(filepath.Join(pkgDir, "xypriss.plugin.sig")); err == nil && !sig.IsDir() {
		i.verifySignatureTrust(filepath.Join(pkgDir, "xypriss.plugin.sig"), pkg)
	}

	if pkg.IsRevoked {
		i.patchPackageJsonForRevocation(pkgDir, pkg)
	}
	return nil
}

func (i *Installer) verifySignatureTrust(sigPath string, pkg *ResolvedPackage) error {
	sigBytes, err := os.ReadFile(sigPath)
	if err != nil { return nil }
	var sigData struct {
		AuthorKey string `json:"author_key"`
	}
	if err := json.Unmarshal(sigBytes, &sigData); err != nil || sigData.AuthorKey == "" { return nil }

	configPath := filepath.Join(i.projectRoot, "xypriss.config.jsonc")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(i.projectRoot, "xypriss.config.json")
	}

	var config map[string]interface{}
	if cfgBytes, err := os.ReadFile(configPath); err == nil {
		lines := strings.Split(string(cfgBytes), "\n")
		var cleanLines []string
		for _, line := range lines {
			if idx := strings.Index(line, "//"); idx != -1 {
				line = line[:idx]
			}
			cleanLines = append(cleanLines, line)
		}
		json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &config)
	}

	if config == nil { config = make(map[string]interface{}) }
	internalRaw, ok := config["$internal"]
	if !ok { internalRaw = make(map[string]interface{}) }
	internal, ok := internalRaw.(map[string]interface{})
	if !ok { internal = make(map[string]interface{}) }

	// 1. Check existing trust in $internal
	if pluginCfg, ok := internal[pkg.Name].(map[string]interface{}); ok {
		if sigCfg, ok := pluginCfg["signature"].(map[string]interface{}); ok {
			if trusted, ok := sigCfg["author_key"].(string); ok && trusted == sigData.AuthorKey { return nil }
		}
	}

	// Pause and hide progress bar to prevent messy UI
	i.barMu.Lock()
	i.isBarPaused = true
	// We don't unlock here yet; we keep it locked to block the ticker goroutine
	
	// Stop progress bar to ensure a clean prompt area.
	// Since WithRemoveWhenDone(true) was set, this will clear the bar from the terminal.
	i.barMu.Lock()
	i.isBarPaused = true
	if i.bar != nil {
		i.bar.Stop()
	}
	i.barMu.Unlock()
	
	pterm.Println()
	pterm.DefaultBox.WithTitle("[SECURITY] New plugin author detected: " + pkg.Name).Println(
		fmt.Sprintf("Package: %s\n\nACTION REQUIRED:\nThis plugin has never been trusted before.\nVerify the Developer ID from the official README:\nhttps://npmjs.com/package/%s\n\nThen paste the Developer ID below to confirm trust.", pkg.Name, pkg.Name),
	)

	// FIX 2: Disable the spinner/loading indicator while waiting for user input.
	result, _ := pterm.DefaultInteractiveTextInput.
		WithDefaultText("").
		Show("Paste the Developer ID to confirm trust, or press Enter to cancel")

	if strings.TrimSpace(result) == sigData.AuthorKey {
		pluginCfgRaw, ok := internal[pkg.Name]
		if !ok { pluginCfgRaw = make(map[string]interface{}) }
		pluginCfg, ok := pluginCfgRaw.(map[string]interface{})
		if !ok { pluginCfg = make(map[string]interface{}) }

		sigCfgRaw, ok := pluginCfg["signature"]
		if !ok { sigCfgRaw = make(map[string]interface{}) }
		sigCfg, ok := sigCfgRaw.(map[string]interface{})
		if !ok { sigCfg = make(map[string]interface{}) }

		sigCfg["author_key"] = sigData.AuthorKey
		pluginCfg["signature"] = sigCfg
		internal[pkg.Name] = pluginCfg
		config["$internal"] = internal

		if out, err := json.MarshalIndent(config, "", "    "); err == nil {
			os.WriteFile(configPath, out, 0644)
			utils.Success("Plugin %s trusted successfully. Developer ID pinned.", pkg.Name)
		}
		
		// Attempt to resume the progress bar if needed? 
		// Actually, starting a new bar is complex and might overlap. 
		// Given we are at the end of the installation usually, it's safer to just continue without the bar.
		return nil
	} else {
		// Even if empty or cancelled via Enter, trigger deletion
		pterm.Println()
		utils.Error("Trust verification failed or cancelled. Removing package %s from disk...", pkg.Name)

		// FIX 3: Remove the untrusted package from the virtual store and root node_modules.
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		vstoreDir := filepath.Join(i.vstoreRoot, pkgVStoreName)
		rootNMDir := filepath.Join(i.projectRoot, "node_modules", pkg.Name)

		os.RemoveAll(vstoreDir)
		os.RemoveAll(rootNMDir)
		
		utils.Info("Package %s has been removed. Trust the author to reinstall.", pkg.Name)
		return fmt.Errorf("security: untrusted plugin author for %s", pkg.Name)
	}
}

func (i *Installer) patchPackageJsonForRevocation(pkgDir string, pkg *ResolvedPackage) {
	pkgJsonPath := filepath.Join(pkgDir, "package.json")
	data, err := os.ReadFile(pkgJsonPath)
	if err != nil { return }

	var pkgJson map[string]interface{}
	if err := json.Unmarshal(data, &pkgJson); err != nil { return }

	xfpmRaw, ok := pkgJson["xfpm"]
	var xfpm map[string]interface{}
	if ok {
		xfpm, _ = xfpmRaw.(map[string]interface{})
	}
	if xfpm == nil {
		xfpm = make(map[string]interface{})
	}

	xfpm["revoked"] = true
	xfpm["revoked_by"] = pkg.RevokedBy
	xfpm["revocation_timestamp"] = time.Now().Format(time.RFC3339)
	
	pkgJson["xfpm"] = xfpm

	// Before writing, we must ensure we are not overwriting a hardlink/reflink from CAS
	// if we don't want to affect the whole system.
	// But in XFPM, we want revoked packages to BE revoked.
	// However, it's cleaner to replace the file.
	os.Remove(pkgJsonPath)
	
	if out, err := json.MarshalIndent(pkgJson, "", "  "); err == nil {
		os.WriteFile(pkgJsonPath, out, 0644)
		utils.Success("Package %s@%s patched with revocation metadata.", pkg.Name, pkg.Version)
	}
}
