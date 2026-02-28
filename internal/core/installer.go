package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
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
	cas         *Cas
	registry    *RegistryClient
	projectRoot string
	vstoreRoot  string
	bar         *pterm.ProgressbarPrinter
	Force       bool
}

func NewInstaller(cas *Cas, registry *RegistryClient, projectRoot string) *Installer {
	vstoreRoot := filepath.Join(projectRoot, "node_modules", ".xpm", "virtual_store")
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
	utils.Success("Installation phase complete.")

	// 5. Run lifecycle scripts
	return i.runLifecycleScripts(ctx, packages)
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
				if rand.Float32() < 0.1 {
					pterm.Printf("   %s %-30s %s\n", utils.GreenColor.Sprint("├─"), p.Name, utils.DimColor.Sprint("extracting..."))
				}
				mu.Lock()
				lastPkg = p.Name
				mu.Unlock()

				if err := i.ensureExtracted(ctx, p); err != nil {
					errChan <- err
				}
				i.bar.Increment()
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
	return nil
}

func (i *Installer) ensureExtracted(ctx context.Context, pkg *ResolvedPackage) error {
	pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
	pkgDir := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)

	if !i.Force {
		if _, err := os.Stat(filepath.Join(pkgDir, "package.json")); err == nil {
			return nil
		}
	} else {
		os.RemoveAll(filepath.Dir(pkgDir)) // Remove entire vstore/pkg@version/node_modules
	}

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

	// Link files from CAS to virtual store
	return i.LinkFilesToDir(pkgDir, fileMap)
}

func (i *Installer) LinkFilesToDir(destDir string, index map[string]string) error {
	os.MkdirAll(destDir, 0755)

	for relPath, hash := range index {
		normalized := relPath
		if strings.HasPrefix(relPath, "package/") {
			normalized = relPath[len("package/"):]
		}

		destPath := filepath.Join(destDir, normalized)
		sourcePath := i.cas.GetFilePath(hash)

		os.MkdirAll(filepath.Dir(destPath), 0755)

		if err := os.Link(sourcePath, destPath); err != nil {
			if err := i.copyFile(sourcePath, destPath); err != nil {
				return err
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

	for _, pkg := range packages {
		rootNM := filepath.Join(i.projectRoot, "node_modules", pkg.Name)
		os.MkdirAll(filepath.Dir(rootNM), 0755)

		pkgVStoreName := strings.ReplaceAll(pkg.Name, "/", "+") + "@" + pkg.Version
		absTarget := filepath.Join(i.vstoreRoot, pkgVStoreName, "node_modules", pkg.Name)

		os.Remove(rootNM)
		if err := os.Symlink(absTarget, rootNM); err != nil {
			// Handle error
		}

		// Also link binaries to project root .bin
		if pkg.Metadata.Bin != nil {
			i.linkBinaries(absTarget, rootBinDir, pkg.Metadata.Bin)
		}
	}
	return nil
}

func (i *Installer) runLifecycleScripts(ctx context.Context, packages []*ResolvedPackage) error {
	runner := NewScriptRunner(i.projectRoot)
	tasks := []ScriptTask{}

	lifecycleOrder := []string{"preinstall", "install", "postinstall"}

	for _, pkg := range packages {
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
