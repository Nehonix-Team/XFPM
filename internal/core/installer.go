package core

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Nehonix-Team/XFMP/internal/paths"
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
	AutoVerify      bool
	NoInteract      bool
	IgnoreScripts   bool
	TargetPackages  map[string]bool
	globalDirCache  sync.Map

	// [OPTIM] Pre-computed OS flags to avoid repeated runtime.GOOS checks
	isWindows bool
	// [OPTIM] Copy buffer pool — reused across copyFile calls
	copyBufPool sync.Pool
}

func NewInstaller(cas *Cas, registry *RegistryClient, projectRoot string) *Installer {
	// Local Virtual Store for Ancestor Hoisting
	vstoreRoot := paths.LocalVStoreDir(projectRoot)
	utils.CreateDirAllSecure(vstoreRoot)

	isWindows := runtime.GOOS == "windows"

	// [OPTIM] On Windows, NTFS + Antivirus latency is the bottleneck, not CPU.
	// Higher concurrency compensates for per-op latency (each syscall is ~5-10x slower).
	// On Linux, lower values avoid inode lock contention.
	maxConcurrency := 128
	if isWindows {
		maxConcurrency = 96 // raised from 48 — NTFS latency needs more parallelism to hide it
	}

	// En mode silencieux, on redirige mpb vers io.Discard pour
	// eliminer tout rendu terminal sans modifier la logique d'incrementation.
	progressOutput := io.Writer(os.Stdout)
	if utils.SilentMode {
		progressOutput = io.Discard
	}

	refreshRate := func() time.Duration {
		if isWindows {
			return 300 * time.Millisecond
		}
		return 180 * time.Millisecond
	}()

	inst := &Installer{
		cas:         cas,
		registry:    registry,
		projectRoot: projectRoot,
		vstoreRoot:  vstoreRoot,
		isWindows:   isWindows,
		linkingPool: utils.NewAdaptiveSemaphore(maxConcurrency),
		progress: mpb.New(
			mpb.WithOutput(progressOutput),
			mpb.WithWidth(64),
			mpb.WithRefreshRate(refreshRate),
		),
	}

	// [OPTIM] Pool of large copy buffers — avoids GC pressure during bulk file copying
	bufSize := 256 * 1024 // 256KB default
	if isWindows {
		bufSize = 1024 * 1024 // 1MB — NTFS benefits from larger sequential writes
	}
	inst.copyBufPool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, bufSize)
			return &buf
		},
	}

	return inst
}

func (i *Installer) ensureDir(path string) {
	ch := make(chan struct{})
	actual, loaded := i.globalDirCache.LoadOrStore(path, ch)
	if loaded {
		// Another worker is already creating this directory.
		// Wait for the 'close(ch)' signal before proceeding.
		<-actual.(chan struct{})
	} else {
		// We are the designated creator for this path in this transaction.
		utils.CreateDirAllSecure(path)
		close(ch)
	}
}

func (i *Installer) Install(ctx context.Context, packages []*ResolvedPackage) (err error) {
	if utils.SilentMode {
		pterm.EnableOutput()
		spinner, _ := pterm.DefaultSpinner.WithText("Installing dependencies...").Start()
		defer func() {
			if err != nil {
				spinner.Fail("Installation failed.")
			} else {
				spinner.Success("Installation complete.")
			}
			pterm.DisableOutput()
		}()
	}

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

	// ALWAYS run lifecycle scripts BEFORE exporting global binaries
	if !i.IgnoreScripts {
		if err := i.runLifecycleScripts(ctx, packages); err != nil {
			return err
		}
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

	// [OPTIM] Windows NTFS has high per-op latency (~1ms vs ~0.1ms on Linux ext4).
	// More concurrent workers hide this latency by keeping the pipeline full.
	// Linux uses fewer workers to avoid inode lock contention on hot directories.
	concurrency := 16
	if i.isWindows {
		concurrency = 24 // raised from 8 — more workers to overlap NTFS latency
	}

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

	if pkg.LocalPath != "" { return i.extractLocal(pkg, targetVStore) }

	if !i.Force {
		if _, err := os.Stat(filepath.Join(pkgDir, "package.json")); err == nil { return nil }
	}


	// [OPTIM] Check Global CAS first before network download
	if !i.Force && i.cas.HasIndex(pkg.Name, pkg.Version) {
		if fileMap, err := i.cas.GetIndex(pkg.Name, pkg.Version); err == nil && len(fileMap) > 0 {
			isPlugin := false
			for path := range fileMap {
				if strings.HasSuffix(path, "xypriss.plugin.xsig") {
					isPlugin = true
					break
				}
			}

			if isPlugin {
				if i.isPluginTrusted(pkg) {
					i.LinkFilesToDir(pkgDir, fileMap)
				} else {
					i.pendingMu.Lock()
					i.PendingPlugins = append(i.PendingPlugins, pkg)
					i.pendingMu.Unlock()
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
	}

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

	// [OPTIM] Pre-collect unique parent directories and create them all up-front
	// before spawning workers. This avoids repeated CreateDirAllSecure calls inside
	// hot goroutines and eliminates the sync.Map lookup overhead per file.
	dirsToCreate := make(map[string]struct{}, len(index))
	type linkJob struct{ relPath, hash string }
	jobs := make([]linkJob, 0, len(index))

	for r, h := range index {
		normalized := r
		if idx := strings.Index(r, "/"); idx != -1 {
			normalized = r[idx+1:]
		}
		destPath := filepath.Join(destDir, normalized)
		parentDir := filepath.Dir(destPath)
		dirsToCreate[parentDir] = struct{}{}
		jobs = append(jobs, linkJob{normalized, h})
	}

	// Batch dir creation in one pass — single goroutine, no lock contention
	for dir := range dirsToCreate {
		i.ensureDir(dir)
	}

	// [OPTIM] Scale workers to file count. Windows: more workers to hide NTFS latency.
	// Linux: fewer to avoid lock contention.
	concurrency := 12
	if i.isWindows {
		concurrency = 20
	}
	if len(jobs) < concurrency {
		concurrency = len(jobs)
	}
	if concurrency < 1 {
		concurrency = 1
	}

	jobCh := make(chan linkJob, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				destPath := filepath.Join(destDir, j.relPath)
				sourcePath := i.cas.GetFilePath(j.hash)

				// [OPTIM] Skip os.Remove if file doesn't exist — avoids a syscall on Windows
				// (os.Remove on a non-existent file still hits the kernel on NTFS)
				if _, err := os.Lstat(destPath); err == nil {
					os.Remove(destPath)
				}

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
	if i.isWindows { return }
	if fi, err := os.Stat(src); err == nil {
		if (fi.Mode() & 0111) != 0 {
			os.Chmod(dst, 0755)
		} else {
			os.Chmod(dst, 0644)
		}
	}
}

func (i *Installer) linkPackageDeps(packages []*ResolvedPackage, vstoreBase string) error {
	// [OPTIM] Replaced unbounded goroutine-per-package with a fixed worker pool.
	// Previously: N goroutines each calling linkingPool.Acquire(128) caused a thundering
	// herd — all goroutines fought for the same semaphore tokens simultaneously.
	// Now: a bounded pool processes packages sequentially per worker, eliminating
	// contention and reducing goroutine scheduling overhead.
	workerCount := 64
	if i.isWindows {
		workerCount = 96 // more workers to saturate NTFS pipeline
	}

	pkgCh := make(chan *ResolvedPackage, len(packages))
	for _, pkg := range packages {
		pkgCh <- pkg
	}
	close(pkgCh)

	errChan := make(chan error, len(packages))
	var wg sync.WaitGroup

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range pkgCh {
				if p.ResolvedDependencies == nil { continue }

				pkgVStoreName := strings.ReplaceAll(p.Name, "/", "+") + "@" + p.Version
				pkgDepsNM := filepath.Join(vstoreBase, pkgVStoreName, "node_modules")

				// [OPTIM] Batch parent dir creation before per-dep work
				parentCreated := false

				for depName, depVersion := range p.ResolvedDependencies {
					realDepName := depName
					if rName, ok := p.DependencyRealNames[depName]; ok {
						realDepName = rName
					}

					targetLink := filepath.Join(pkgDepsNM, depName)

					// COLLISION GUARD: Though each unique version has its own vstore NM,
					// if we process uniquePackages, we only ever visit each once.
					if _, seen := i.linkedPaths.LoadOrStore(targetLink, true); seen {
						continue
					}

					// [OPTIM] Create parent dir once per package, not per dep
					if !parentCreated {
						i.ensureDir(pkgDepsNM)
						parentCreated = true
					}

					// For scoped packages the parent dir differs
					parentDir := filepath.Dir(targetLink)
					if parentDir != pkgDepsNM {
						i.ensureDir(parentDir)
					}

					depVStoreName := strings.ReplaceAll(realDepName, "/", "+") + "@" + depVersion
					steps := strings.Count(depName, "/") + 2
					relPrefix := strings.Repeat("../", steps)
					relTarget := filepath.Join(relPrefix, depVStoreName, "node_modules", realDepName)

					// [OPTIM] Skip os.Remove if link doesn't exist
					if _, err := os.Lstat(targetLink); err == nil {
						os.Remove(targetLink)
					}
					if err := utils.Link(relTarget, targetLink); err != nil {
						utils.Error("Failed to link dependency %s -> %s: %v", depName, targetLink, err)
					}
				}
			}
		}()
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
		if _, seen := i.linkedPaths.LoadOrStore(rootNM, true); seen {
			return
		}

		if err := i.linkingPool.Acquire(context.Background(), 128); err != nil {
			return
		}
		defer i.linkingPool.Release()

		parentDir := filepath.Dir(rootNM)
		i.ensureDir(parentDir)
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version

		absTarget := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		relTarget, _ := filepath.Rel(filepath.Dir(rootNM), absTarget)

		// [OPTIM] Lstat before Remove
		if _, err := os.Lstat(rootNM); err == nil {
			os.Remove(rootNM)
		}
		if err := utils.Link(relTarget, rootNM); err != nil {
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
		if i.TargetPackages != nil && !i.TargetPackages[pkg.Name] {
			continue
		}
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
	// CLEAN RELATIVE PATH (Handle .exe suffixes in Unix or nested bins)
	cleanRel := strings.TrimSuffix(relPath, ".exe")
	if i.isWindows && !strings.HasSuffix(cleanRel, ".exe") {
		cleanRel += ".exe"
	}

	absTarget := filepath.Join(pkgDir, cleanRel)

	// ADAPTIVE DISCOVERY
	if _, err := os.Stat(absTarget); os.IsNotExist(err) {
		absTarget = filepath.Join(pkgDir, relPath)
		if _, err := os.Stat(absTarget); os.IsNotExist(err) {
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

	dest := filepath.Join(binDir, name)
	if i.isWindows && strings.HasSuffix(strings.ToLower(absTarget), ".exe") {
		if !strings.HasSuffix(strings.ToLower(dest), ".exe") {
			dest += ".exe"
		}
	}

	// COLLISION GUARD: First binary to claim the name wins
	if _, seen := i.linkedPaths.LoadOrStore(dest, true); seen {
		return
	}
	// [OPTIM] Lstat before Remove
	if _, err := os.Lstat(dest); err == nil {
		os.Remove(dest)
	}

	relTarget, _ := filepath.Rel(binDir, absTarget)
	if !i.isWindows {
		os.Symlink(relTarget, dest)
		os.Chmod(absTarget, 0755)
		i.ensureExecutableRecursive(filepath.Dir(absTarget))
	} else {
		if err := utils.Link(relTarget, dest); err != nil {
			utils.Error("Failed to link binary %s -> %s: %v", name, dest, err)
		}
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
	globalBinDir := paths.BinDir()
	utils.CreateDirAllSecure(globalBinDir)

	for _, pkg := range packages {
		if _, isDirect := i.DirectDeps[pkg.Name]; !isDirect { continue }
		if pkg.Metadata.Bin == nil { continue }
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		i.linkBinaries(pkgDir, globalBinDir, pkg.Metadata.Bin)
	}
}

// [OPTIM] copyFile uses a pooled buffer (256KB on Linux, 1MB on Windows).
// The default io.Copy uses 32KB which is too small for NTFS sequential writes.
func (i *Installer) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()

	// [OPTIM] Lstat before Remove — avoids a kernel call when file doesn't exist
	if _, err := os.Lstat(dst); err == nil {
		os.Remove(dst)
	}
	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()

	bufPtr := i.copyBufPool.Get().(*[]byte)
	defer i.copyBufPool.Put(bufPtr)
	_, err = io.CopyBuffer(out, in, *bufPtr)
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
	// i.cas.StoreIndex(pkg.Name, pkg.Version, fileMap) // DO NOT cache index for local installs to prevent poisoning NPM cache
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
	configPath := paths.ConfigPath(i.projectRoot)

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
	configStr := strings.Join(cleanLines, "\n")
	re := regexp.MustCompile(`,(\s*[}\]])`)
	configStr = re.ReplaceAllString(configStr, "$1")
	if err := json.Unmarshal([]byte(configStr), &config); err != nil { return false }

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
		AuthorKey  string `json:"author_key"`
		Privileges string `json:"privileges"`
	}

	sigStr := string(sigBytes)
	if strings.Contains(sigStr, "--- XYPRISS SIGNATURE (G3) ---") {
		// G3 Signature Parser (Zero-Trust G3)
		headers := make(map[string]string)
		var sigBody strings.Builder
		var proofBase64 string
		inProof := false

		scanner := bufio.NewScanner(strings.NewReader(sigStr))
		for scanner.Scan() {
			line := scanner.Text()
			if line == "--- BEGIN CRYPTOGRAPHIC PROOF ---" {
				inProof = true
				continue
			}
			if strings.HasPrefix(line, "--- END") {
				break
			}

			if inProof {
				if strings.HasPrefix(line, "base64:") {
					proofBase64 = strings.TrimPrefix(line, "base64:")
				}
				continue
			}

			sigBody.WriteString(line + "\n")

			if idx := strings.Index(line, ": "); idx != -1 {
				key := line[:idx]
				val := line[idx+2:]
				headers[key] = val
			}
		}

		identity, ok := headers["Identity"]
		privileges, _ := headers["Privileges"]
		if !ok || proofBase64 == "" {
			return fmt.Errorf("security: invalid or corrupted G3 signature in package %s", pkg.Name)
		}

		// Cryptographic Proof Verification
		if strings.HasPrefix(identity, "ed25519:") {
			pubKeyHex := strings.TrimPrefix(identity, "ed25519:")
			pubKey, err := hex.DecodeString(pubKeyHex)
			if err != nil {
				return fmt.Errorf("security: invalid identity format in G3 signature")
			}

			sig, err := base64.StdEncoding.DecodeString(proofBase64)
			if err != nil {
				return fmt.Errorf("security: corrupted cryptographic proof in G3 signature")
			}

			if !ed25519.Verify(pubKey, []byte(sigBody.String()), sig) {
				return fmt.Errorf("security: cryptographic verification failed for package %s", pkg.Name)
			}
		}

		sigData.AuthorKey = identity
		sigData.Privileges = privileges
	} else {
		// Legacy JSON format
		if err := json.Unmarshal(sigBytes, &sigData); err != nil || sigData.AuthorKey == "" {
			return fmt.Errorf("security: invalid or corrupted signature in package %s", pkg.Name)
		}
	}

	configPath := paths.ConfigPath(i.projectRoot)

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
		configStr := strings.Join(cleanLines, "\n")
		re := regexp.MustCompile(`,(\s*[}\]])`)
		configStr = re.ReplaceAllString(configStr, "$1")
		json.Unmarshal([]byte(configStr), &config)
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
	privStr := ""
	if sigData.Privileges != "" && sigData.Privileges != "none" {
		privStr = fmt.Sprintf("\n\nREQUESTED PRIVILEGES:\n%s", strings.ReplaceAll(sigData.Privileges, ",", "\n"))
	}

	pterm.DefaultBox.WithTitle("[SECURITY] New plugin author detected: " + pkg.Name).Println(
		fmt.Sprintf("Package: %s\n\nACTION REQUIRED:\nThis plugin has never been trusted before.\nVerify the Developer ID from the official README:\nhttps://npmjs.com/package/%s%s\n\nThen paste the Developer ID below to confirm trust (and authorize all privileges).", pkg.Name, pkg.Name, privStr),
	)

	// Check package.json for trustedPlugins
	isTrustedInPkgJson := false
	if pkgJson, err := LoadPackageJson(filepath.Join(i.projectRoot, "package.json")); err == nil && pkgJson != nil {
		if pkgJson.Xfpm != nil {
			for _, item := range pkgJson.Xfpm.TrustedPlugins {
				if item == pkg.Name {
					isTrustedInPkgJson = true
					break
				}
			}
		}
	}

	var result string
	if i.NoInteract || isTrustedInPkgJson {
		if isTrustedInPkgJson {
			utils.Info("Auto-verifying plugin %s (Whitelisted in package.json)...", pkg.Name)
		}
		result = sigData.AuthorKey
	} else if !i.NoInteract {
		result, _ = pterm.DefaultInteractiveTextInput.
			WithDefaultText("").
			Show("Paste the Developer ID to confirm trust, or press Enter to cancel")
	}

	if i.NoInteract || isTrustedInPkgJson || strings.TrimSpace(result) == sigData.AuthorKey {
		if i.NoInteract {
			utils.Info("Auto-verifying plugin %s (Non-interactive mode)...", pkg.Name)
		}
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

		if sigData.Privileges != "" && sigData.Privileges != "none" {
			permCfgRaw, ok := pluginCfg["permissions"]
			if !ok { permCfgRaw = make(map[string]interface{}) }
			permCfg, ok := permCfgRaw.(map[string]interface{})
			if !ok { permCfg = make(map[string]interface{}) }

			permCfg["allowedHooks"] = strings.Split(sigData.Privileges, ",")
			pluginCfg["permissions"] = permCfg
		}

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

	if i.AutoVerify {
		var verifiedCount int
		var stillPending []*ResolvedPackage

		for _, p := range i.PendingPlugins {
			pkgDir := paths.PackageVStoreDir(i.projectRoot, p.Name, p.Version)
			sigPath := paths.PackageSigPath(pkgDir)

			// Load index from CAS to verify
			index, err := i.cas.GetIndex(p.Name, p.Version)
			if err != nil {
				utils.Error("Package index not found in CAS for %s. Skipping auto-verify.", p.Name)
				stillPending = append(stillPending, p)
				continue
			}

			if err := i.VerifySignatureInternal(sigPath, p, index); err == nil {
				// Finalize installation for this plugin
				i.LinkFilesToDir(pkgDir, index)
				rootDest := filepath.Join(i.projectRoot, "node_modules", p.Name)
				utils.Link(pkgDir, rootDest)
				verifiedCount++
			} else {
				utils.Error("Verification failed for %s: %v", p.Name, err)
				stillPending = append(stillPending, p)
			}
		}

		i.PendingPlugins = stillPending
		if verifiedCount > 0 {
			utils.Success("Auto-verified and finalized %d plugin(s).", verifiedCount)
		}
		if len(i.PendingPlugins) == 0 { return }
	}

	pendingPath := paths.PendingPluginsPath(i.projectRoot)
	utils.CreateDirAllSecure(filepath.Dir(pendingPath))

	var list []map[string]string
	for _, p := range i.PendingPlugins {
		list = append(list, map[string]string{
			"name":    p.Name,
			"version": p.Version,
		})
	}

	data, _ := json.MarshalIndent(list, "", "  ")
	os.WriteFile(pendingPath, data, 0644)

	pterm.Println()
	utils.Warn("  ⚠️  [SECURITY] %d plugin(s) pending verification.", len(i.PendingPlugins))
	utils.Info("      Run 'xfpm plugin verify -w' to finalize installation.")
}

func (i *Installer) isPluginTrusted(pkg *ResolvedPackage) bool {
	configPath := paths.ConfigPath(i.projectRoot)

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
	configStr := strings.Join(cleanLines, "\n")
	re := regexp.MustCompile(`,(\s*[}\]])`)
	configStr = re.ReplaceAllString(configStr, "$1")
	json.Unmarshal([]byte(configStr), &config)

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