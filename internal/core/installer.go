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
	ForcePackages   map[string]bool
	IsGlobal        bool
	DirectDeps      map[string]string
	changedPackages sync.Map
}

func NewInstaller(cas *Cas, registry *RegistryClient, projectRoot string) *Installer {
	home, _ := os.UserHomeDir()
	vstoreRoot := filepath.Join(home, ".xpm", "virtual_store")
	os.MkdirAll(vstoreRoot, 0755)

	return &Installer{
		cas:         cas,
		registry:    registry,
		projectRoot: projectRoot,
		vstoreRoot:  vstoreRoot,
	}
}

func (i *Installer) isShadowPackage(pkg *ResolvedPackage) bool {
	if pkg.Metadata.Scripts != nil {
		if _, ok := pkg.Metadata.Scripts["install"]; ok { return true }
		if _, ok := pkg.Metadata.Scripts["postinstall"]; ok { return true }
		if _, ok := pkg.Metadata.Scripts["preinstall"]; ok { return true }
	}
	if strings.Contains(pkg.Name, "prisma") || strings.Contains(pkg.Name, "koffi") {
		return true
	}
	return false
}

func (i *Installer) Install(ctx context.Context, packages []*ResolvedPackage) error {
	utils.Info("Starting installation of %d packages...", len(packages))

	shadowRoot := filepath.Join(i.projectRoot, "node_modules", ".xpm", "shadow")
	os.MkdirAll(shadowRoot, 0755)

	i.bar, _ = pterm.DefaultProgressbar.
		WithTotal(len(packages)).
		WithTitle(utils.MatrixColor.Sprint("  [INIT_SEQ]")).
		WithBarCharacter("█").
		WithLastCharacter("█").
		WithRemoveWhenDone(true).
		Start()

	var shadowed []*ResolvedPackage
	var globals []*ResolvedPackage

	for _, pkg := range packages {
		if i.isShadowPackage(pkg) {
			shadowed = append(shadowed, pkg)
		} else {
			globals = append(globals, pkg)
		}
	}

	uniqueGlobals := i.getUniquePackages(globals)
	uniqueShadowed := i.getUniquePackages(shadowed)

	if err := i.batchEnsureExtracted(ctx, uniqueGlobals, i.vstoreRoot); err != nil {
		i.bar.Stop()
		return err
	}
	if err := i.batchEnsureExtracted(ctx, uniqueShadowed, shadowRoot); err != nil {
		i.bar.Stop()
		return err
	}

	if err := i.linkPackageDeps(globals, i.vstoreRoot, true); err != nil {
		i.bar.Stop()
		return err
	}
	if err := i.linkPackageDeps(shadowed, shadowRoot, false); err != nil {
		i.bar.Stop()
		return err
	}

	if err := i.batchLinkBinaries(packages); err != nil {
		i.bar.Stop()
		return err
	}

	if err := i.linkToRoot(packages, shadowRoot); err != nil {
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

	if err := i.runLifecycleScripts(ctx, packages); err != nil {
		return err
	}

	if i.IsGlobal {
		i.exportGlobalBinaries(packages)
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
	stopMatrix := make(chan struct{})
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		var lastBytes uint64 = GlobalBytesDownloaded.Load()
		for {
			select {
			case <-ticker.C:
				currentBytes := GlobalBytesDownloaded.Load()
				diff := currentBytes - lastBytes
				lastBytes = currentBytes
				speedBytesPerSec := diff * 5
				var speedStr string
				if speedBytesPerSec > 0 { speedStr = " [" + utils.FormatBytes(speedBytesPerSec) + "/s]" }
				mu.Lock()
				pkg := lastPkg
				mu.Unlock()
				if pkg != "" {
					elapsed := time.Since(startTime).Truncate(time.Millisecond)
					i.bar.UpdateTitle(fmt.Sprintf("  %s %s%s %s  ", pterm.FgCyan.Sprint("[EXTRACTING]"), utils.AccentColor.Sprint(elapsed.String()), pterm.FgYellow.Sprint(speedStr), utils.DimColor.Sprint(pkg)))
				}
			case <-stopMatrix: return
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
				mu.Lock()
				i.bar.Increment()
				mu.Unlock()
			case <-ctx.Done(): return
			}
		}(pkg)
	}
	wg.Wait()
	close(stopMatrix)
	close(errChan)
	if len(errChan) > 0 { return <-errChan }
	i.bar.UpdateTitle("  " + pterm.BgBlue.Sprint(" SUCCESS ") + "  " + utils.GreenColor.Sprint("Extraction phase complete.") + "  ")
	return nil
}

func (i *Installer) ensureExtracted(ctx context.Context, pkg *ResolvedPackage, targetVStore string) error {
	if pkg.LocalPath != "" { return i.extractLocal(pkg, targetVStore) }
	pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
	pkgDir := filepath.Join(targetVStore, pkgVStoreName, "node_modules", pkg.Name)

	if !i.Force {
		if _, err := os.Stat(filepath.Join(pkgDir, "package.json")); err == nil { return nil }
	} else {
		cacheKey := pkg.Name + "@" + pkg.Version
		if _, exists := i.changedPackages.Load(cacheKey); !exists { os.RemoveAll(pkgDir) }
	}

	pterm.Printf("   %s %-30s %s\n", utils.GreenColor.Sprint("├─"), pkg.Name+"@"+pkg.Version, utils.DimColor.Sprint("extracting..."))
	stream, err := i.registry.DownloadTarballStream(ctx, pkg.Metadata.Dist.Tarball)
	if err != nil { return err }
	defer stream.Close()

	extractor := NewStreamingExtractor(i.cas)
	fileMap, err := extractor.Extract(stream)
	if err != nil { return err }

	i.cas.StoreIndex(pkg.Name, pkg.Version, fileMap)
	i.LinkFilesToDir(pkgDir, fileMap)
	i.changedPackages.Store(pkg.Name+"@"+pkg.Version, true)
	return nil
}

func (i *Installer) LinkFilesToDir(destDir string, index map[string]string) error {
	os.MkdirAll(destDir, 0755)
	for relPath, hash := range index {
		normalized := relPath
		if idx := strings.Index(relPath, "/"); idx != -1 { normalized = relPath[idx+1:] }
		destPath := filepath.Join(destDir, normalized)
		sourcePath := i.cas.GetFilePath(hash)
		os.MkdirAll(filepath.Dir(destPath), 0755)
		os.Remove(destPath)
		if err := utils.Reflink(sourcePath, destPath); err == nil {
			if fi, err := os.Stat(sourcePath); err == nil && (fi.Mode()&0111) != 0 { os.Chmod(destPath, 0755) } else { os.Chmod(destPath, 0644) }
			continue
		}
		if err := os.Link(sourcePath, destPath); err == nil { continue }
		i.copyFile(sourcePath, destPath)
	}
	return nil
}

func (i *Installer) linkPackageDeps(packages []*ResolvedPackage, vstoreBase string, useRelative bool) error {
	for _, pkg := range packages {
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDepsNM := filepath.Join(vstoreBase, pkgVStoreName, "node_modules")
		for depName, depVersion := range pkg.ResolvedDependencies {
			targetLink := filepath.Join(pkgDepsNM, depName)
			os.MkdirAll(filepath.Dir(targetLink), 0755)
			depVStoreName := strings.ReplaceAll(depName, "/", "+") + "@" + depVersion
			if useRelative {
				steps := strings.Count(depName, "/") + 2
				relPrefix := ""
				for j := 0; j < steps; j++ { relPrefix += "../" }
				relTarget := filepath.Join(relPrefix, depVStoreName, "node_modules", depName)
				os.Remove(targetLink)
				os.Symlink(relTarget, targetLink)
			} else {
				globalDepTarget := filepath.Join(i.vstoreRoot, depVStoreName, "node_modules", depName)
				os.Remove(targetLink)
				os.Symlink(globalDepTarget, targetLink)
			}
		}
	}
	return nil
}

func (i *Installer) batchLinkBinaries(packages []*ResolvedPackage) error {
	shadowRoot := filepath.Join(i.projectRoot, "node_modules", ".xpm", "shadow")
	for _, pkg := range packages {
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		var pkgAbsPath string
		var targetBinDir string
		if i.isShadowPackage(pkg) {
			pkgAbsPath = filepath.Join(shadowRoot, pkgVStoreName, "node_modules", pkg.Name)
			targetBinDir = filepath.Join(shadowRoot, pkgVStoreName, "node_modules", ".bin")
		} else {
			pkgAbsPath = filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
			targetBinDir = filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", ".bin")
		}
		i.linkBinaries(pkgAbsPath, targetBinDir, pkg.Metadata.Bin)
	}
	return nil
}

func (i *Installer) linkBinaries(pkgDir, binDir string, bin json.RawMessage) error {
	if bin == nil { return nil }
	os.MkdirAll(binDir, 0755)
	type BinObject map[string]string
	var binMap BinObject
	var binStr string
	if err := json.Unmarshal(bin, &binMap); err == nil {
		for name, relPath := range binMap { i.createBinLink(name, pkgDir, relPath, binDir) }
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
	if err != nil { relTarget = absTarget }
	if runtime.GOOS != "windows" {
		os.Symlink(relTarget, dest)
		os.Chmod(absTarget, 0755)
	} else {
		os.Symlink(relTarget, dest)
	}
}

func (i *Installer) linkToRoot(packages []*ResolvedPackage, shadowRoot string) error {
	rootNMDir := filepath.Join(i.projectRoot, "node_modules")
	rootBinDir := filepath.Join(rootNMDir, ".bin")
	os.MkdirAll(rootBinDir, 0755)
	directNames := make(map[string]bool)
	for name := range i.DirectDeps { directNames[name] = true }
	linkPkg := func(pkg *ResolvedPackage) {
		rootNM := filepath.Join(rootNMDir, pkg.Name)
		os.MkdirAll(filepath.Dir(rootNM), 0755)
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		var absTarget string
		if i.isShadowPackage(pkg) {
			absTarget = filepath.Join(shadowRoot, pkgVStoreName, "node_modules", pkg.Name)
		} else {
			absTarget = filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		}
		os.Remove(rootNM)
		os.Symlink(absTarget, rootNM)
		if pkg.Metadata.Bin != nil { i.linkBinaries(absTarget, rootBinDir, pkg.Metadata.Bin) }
	}
	for _, pkg := range packages { if !directNames[pkg.Name] { linkPkg(pkg) } }
	for _, pkg := range packages {
		if v, ok := i.DirectDeps[pkg.Name]; ok && pkg.Version == v { linkPkg(pkg) }
	}
	return nil
}

func (i *Installer) runLifecycleScripts(ctx context.Context, packages []*ResolvedPackage) error {
	runner := NewScriptRunner(i.projectRoot)
	tasks := []ScriptTask{}
	shadowRoot := filepath.Join(i.projectRoot, "node_modules", ".xpm", "shadow")
	lifecycleOrder := []string{"preinstall", "install", "postinstall"}
	for _, pkg := range packages {
		cacheKey := pkg.Name + "@" + pkg.Version
		if _, wasChanged := i.changedPackages.Load(cacheKey); !wasChanged { continue }
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		var pkgDir string
		if i.isShadowPackage(pkg) {
			pkgDir = filepath.Join(shadowRoot, pkgVStoreName, "node_modules", pkg.Name)
		} else {
			pkgDir = filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		}
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

func (i *Installer) exportGlobalBinaries(packages []*ResolvedPackage) {
	binDir := filepath.Join(i.projectRoot, "bin")
	os.MkdirAll(binDir, 0755)
	for _, pkg := range packages {
		if _, isDirect := i.DirectDeps[pkg.Name]; !isDirect { continue }
		if pkg.Metadata.Bin == nil { continue }
		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)
		i.linkBinaries(pkgDir, binDir, pkg.Metadata.Bin)
	}
	ensureGlobalPath(binDir)
}

func (i *Installer) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()
	os.Remove(dst)
	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()
	io.Copy(out, in)
	return nil
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
			// (Preserving minimal filtering logic)
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
	return nil
}

func ensureGlobalPath(binDir string) {
	exportLine := fmt.Sprintf("\nexport PATH=\"%s:$PATH\"", binDir)
	home, _ := os.UserHomeDir()
	rcFiles := []string{filepath.Join(home, ".zshrc"), filepath.Join(home, ".bashrc"), filepath.Join(home, ".profile")}
	for _, rc := range rcFiles {
		data, err := os.ReadFile(rc)
		if err != nil { continue }
		if strings.Contains(string(data), binDir) { continue }
		f, _ := os.OpenFile(rc, os.O_APPEND|os.O_WRONLY, 0644)
		f.WriteString(exportLine)
		f.Close()
	}
}
