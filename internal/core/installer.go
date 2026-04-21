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
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type Installer struct {
	cas             *Cas
	registry        *RegistryClient
	projectRoot     string
	vstoreRoot      string // node_modules/.xpm/vstore
	globalBar       *mpb.Bar
	Force           bool
	ForcePackages   map[string]bool
	IsGlobal        bool
	DirectDeps      map[string]string
	changedPackages sync.Map
	linkingPool     *utils.AdaptiveSemaphore
	linkedPaths     sync.Map // string -> bool (to avoid race collisions)
	barMu           sync.Mutex
	isBarPaused     bool
	PendingPlugins  []*ResolvedPackage
	pendingMu       sync.Mutex
	progress        *mpb.Progress
	lastPkg         string
	lpMu            sync.Mutex
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
		progress: mpb.New(
			mpb.WithWidth(64),
			mpb.WithRefreshRate(180*time.Millisecond),
		),
	}
}

func (i *Installer) Install(ctx context.Context, packages []*ResolvedPackage) error {
	utils.Info("Starting installation of %d packages via Ancestor Hoisting...", len(packages))

	uniquePackages := i.getUniquePackages(packages)
	startTime := time.Now()
	initialBytes := GlobalBytesDownloaded.Load()

	i.globalBar, _ = i.progress.Add(int64(len(uniquePackages)),
		mpb.BarFillerFunc(func(w io.Writer, st decor.Statistics) error {
			percent := float64(st.Current) / float64(st.Total) * 100
			color := pterm.FgCyan
			if percent > 50 { color = pterm.FgYellow }
			if percent > 90 { color = pterm.FgGreen }
			
			width := 40
			filled := int(float64(width) * float64(percent) / 100)
			if filled > width { filled = width }
			
			bar := color.Sprint(strings.Repeat("█", filled))
			if filled < width {
				bar += utils.DimColor.Sprint(strings.Repeat("░", width-filled))
			}
			fmt.Fprintf(w, "[%s]", bar)
			return nil
		}),
		mpb.PrependDecorators(
			decor.Name(utils.MatrixColor.Sprint("  [INIT_SEQ] ")),
			decor.CountersNoUnit("[%d/%d] "),
			decor.Any(func(s decor.Statistics) string {
				i.lpMu.Lock()
				pkg := i.lastPkg
				i.lpMu.Unlock()
				if pkg == "" { return "" }
				return utils.DimColor.Sprint("(") + utils.AccentColor.Sprint(pkg) + utils.DimColor.Sprint(") ")
			}),
		),
		mpb.AppendDecorators(
			decor.Name(" "),
			decor.Elapsed(decor.ET_STYLE_GO),
			decor.Name("  "),
			decor.Percentage(),
			decor.Name("  "),
			decor.Any(func(s decor.Statistics) string {
				speedNano := GlobalBytesDownloaded.Load() - initialBytes
				elapsed := time.Since(startTime).Seconds()
				if elapsed < 1 { elapsed = 1 }
				mbps := float64(speedNano) / elapsed / 1024 / 1024
				return utils.AccentColor.Sprint(fmt.Sprintf("% .1f MB/s", mbps))
			}),
		),
		mpb.BarPriority(1000),
	)

	if err := i.batchEnsureExtracted(ctx, uniquePackages, i.vstoreRoot); err != nil {
		return err
	}

	if err := i.linkPackageDeps(uniquePackages, i.vstoreRoot); err != nil {
		return err
	}

	if err := i.linkToRoot(uniquePackages); err != nil {
		return err
	}

	i.globalBar.SetTotal(int64(len(uniquePackages)), true)
	i.progress.Wait()

	// UX: Aggressive clear screen after installation progress
	fmt.Print("\033[H\033[2J")
	// utils.PrintBanner()
	
	// ALWAYS run lifecycle scripts BEFORE exporting global binaries
	if err := i.runLifecycleScripts(ctx, packages); err != nil {
		return err
	}

	if i.IsGlobal {
		i.exportGlobalBinaries(packages)
		utils.EnsurePathInShell()
	}

	i.SavePendingPlugins()

	// Final success message after EVERYTHING is done
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

	return nil
}

func (i *Installer) startNetworkMonitoring() {
	// Logic merged into globalBar decorator
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
	
	i.startNetworkMonitoring()

	for _, pkg := range packages {
		wg.Add(1)
		go func(p *ResolvedPackage) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				i.lpMu.Lock()
				i.lastPkg = p.Name
				i.lpMu.Unlock()
				if err := i.ensureExtracted(ctx, p, targetVStore); err != nil { errChan <- err }
				i.globalBar.Increment()
			case <-ctx.Done(): return
			}
		}(pkg)
	}
	wg.Wait()
	return nil
}

func (i *Installer) ensureExtracted(ctx context.Context, pkg *ResolvedPackage, targetVStore string) error {
	pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
	pkgDir := filepath.Join(targetVStore, pkgVStoreName, "node_modules", pkg.Name)

	if !i.Force {
		if _, err := os.Stat(filepath.Join(pkgDir, "package.json")); err == nil { return nil }
	}

	if pkg.LocalPath != "" { return i.extractLocal(pkg, targetVStore) }

	total := pkg.Metadata.Dist.UnpackedSize
	if total == 0 { total = 100 } // fallback

	pBar := i.progress.AddBar(int64(total),
		mpb.BarPriority(0), // Extraction bars at the top
		mpb.BarRemoveOnComplete(),
		mpb.PrependDecorators(
			decor.Name(utils.DimColor.Sprint("   [EXTRACTING] ")),
			decor.Name(utils.AccentColor.Sprint(pkg.Name)),
		),
		mpb.AppendDecorators(
			decor.Counters(decor.SizeB1024(0), "% .1f / % .1f"),
			decor.Percentage(),
		),
	)
	defer pBar.Abort(true)

	stream, err := i.registry.DownloadTarballStream(ctx, pkg.Metadata.Dist.Tarball)
	if err != nil { return err }
	defer stream.Close()

	// Wrap stream for progress reporting
	progressStream := pBar.ProxyReader(stream)
	defer progressStream.Close()

	extractor := NewStreamingExtractor(i.cas)
	fileMap, err := extractor.Extract(progressStream)
	if err != nil { return err }

	i.cas.StoreIndex(pkg.Name, pkg.Version, fileMap)
	
	// Plugin Detection
	isPlugin := false
	for path := range fileMap {
		if strings.HasSuffix(path, "xypriss.plugin.xsig") {
			isPlugin = true
			break
		}
	}

	if isPlugin {
		// Check if already trusted
		if i.isPluginTrusted(pkg) {
			i.LinkFilesToDir(pkgDir, fileMap)
		} else {
			i.pendingMu.Lock()
			i.PendingPlugins = append(i.PendingPlugins, pkg)
			i.pendingMu.Unlock()
			// We skip Linking for now
			return nil
		}
	} else {
		i.LinkFilesToDir(pkgDir, fileMap)
	}

	i.changedPackages.Store(pkg.Name+"@"+pkg.Version, true)

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

	if sig, err := os.Stat(filepath.Join(pkgDir, "xypriss.plugin.xsig")); err == nil && !sig.IsDir() {
		i.VerifySignatureInternal(filepath.Join(pkgDir, "xypriss.plugin.xsig"), pkg, fileMap)
	}

	if pkg.IsRevoked {
		i.patchPackageJsonForRevocation(pkgDir, pkg)
	}
	return nil
}

func (i *Installer) IsPluginTrustedDirect(pkgName string) bool {
	configPath := filepath.Join(i.projectRoot, "xypriss.config.jsonc")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(i.projectRoot, "xypriss.config.json")
	}

	cfgBytes, err := os.ReadFile(configPath)
	if err != nil { return false }

	lines := strings.Split(string(cfgBytes), "\n")
	var cleanLines []string
	for _, line := range lines {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
		}
		cleanLines = append(cleanLines, line)
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &config); err != nil { return false }

	internal, ok := config["$internal"].(map[string]interface{})
	if !ok { return false }

	pluginCfg, ok := internal[pkgName].(map[string]interface{})
	if !ok { return false }

	sigCfg, ok := pluginCfg["signature"].(map[string]interface{})
	if !ok { return false }

	authorKey, ok := sigCfg["author_key"].(string)
	return ok && authorKey != ""
}

func (i *Installer) VerifySignatureInternal(sigPath string, pkg *ResolvedPackage, index map[string]string) error {
	var sigBytes []byte
	var err error

	if sigPath != "" {
		sigBytes, err = os.ReadFile(sigPath)
	}
	
	if err != nil || len(sigBytes) == 0 {
		// Fallback to CAS if index is provided
		for path, hash := range index {
			if strings.HasSuffix(path, "xypriss.plugin.xsig") {
				casPath := i.cas.GetFilePath(hash)
				sigBytes, _ = os.ReadFile(casPath)
				break
			}
		}
	}

	if len(sigBytes) == 0 { 
		if index != nil {
			return fmt.Errorf("security: signature file 'xypriss.plugin.xsig' missing from package %s", pkg.Name)
		}
		return nil 
	}
	
	var sigData struct {
		AuthorKey string `json:"author_key"`
	}
	if err := json.Unmarshal(sigBytes, &sigData); err != nil || sigData.AuthorKey == "" {
		return fmt.Errorf("security: invalid or corrupted signature in package %s", pkg.Name)
	}

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

	pterm.Println()
	pterm.DefaultBox.WithTitle("[SECURITY] New plugin author detected: " + pkg.Name).Println(
		fmt.Sprintf("Package: %s\n\nACTION REQUIRED:\nThis plugin has never been trusted before.\nVerify the Developer ID from the official README:\nhttps://npmjs.com/package/%s\n\nThen paste the Developer ID below to confirm trust.", pkg.Name, pkg.Name),
	)

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
		return nil
	} else {
		pterm.Println()
		utils.Error("Trust verification failed or cancelled for %s.", pkg.Name)
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

func (i *Installer) SavePendingPlugins() {
	i.pendingMu.Lock()
	defer i.pendingMu.Unlock()
	if len(i.PendingPlugins) == 0 { return }
	
	pendingPath := filepath.Join(i.projectRoot, "node_modules", ".xpm", "pending_plugins.json")
	utils.CreateDirAllSecure(filepath.Dir(pendingPath))

	var list []map[string]string
	for _, p := range i.PendingPlugins {
		list = append(list, map[string]string{
			"name": p.Name,
			"version": p.Version,
		})
	}
	
	data, _ := json.MarshalIndent(list, "", "  ")
	os.WriteFile(pendingPath, data, 0644)
	
	pterm.Println()
	utils.Warn("  ⚠️  [SECURITY] %d plugin(s) pending verification.", len(i.PendingPlugins))
	utils.Info("      Run 'xfpm plugin verify' to finalize installation.")
}

func (i *Installer) isPluginTrusted(pkg *ResolvedPackage) bool {
	configPath := filepath.Join(i.projectRoot, "xypriss.config.jsonc")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(i.projectRoot, "xypriss.config.json")
	}

	cfgBytes, err := os.ReadFile(configPath)
	if err != nil { return false }

	lines := strings.Split(string(cfgBytes), "\n")
	var cleanLines []string
	for _, line := range lines {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
		}
		cleanLines = append(cleanLines, line)
	}

	var config map[string]interface{}
	json.Unmarshal([]byte(strings.Join(cleanLines, "\n")), &config)

	if internalRaw, ok := config["$internal"]; ok {
		if internal, ok := internalRaw.(map[string]interface{}); ok {
			if pluginCfg, ok := internal[pkg.Name].(map[string]interface{}); ok {
				if sigCfg, ok := pluginCfg["signature"].(map[string]interface{}); ok {
					if _, ok := sigCfg["author_key"].(string); ok { return true }
				}
			}
		}
	}
	return false
}
