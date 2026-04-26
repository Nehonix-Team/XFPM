package plugin

import (
	"fmt"
	"path/filepath"

	"github.com/Nehonix-Team/XFMP/internal/core"
	"github.com/Nehonix-Team/XFMP/internal/paths"
	"github.com/Nehonix-Team/XFMP/internal/utils"
)

// InstallPendingPlugin executes the verification and local installation of a pending plugin
// by downloading/extracting from CAS, validating signature integrity, and linking to node_modules.
func InstallPendingPlugin(projectRoot, pkgName, pkgVer string) error {
	casManager, _ := core.NewCas(paths.StorageDir())
	registry := core.NewRegistryClient("https://registry.npmjs.org", 8)
	inst := core.NewInstaller(casManager, registry, projectRoot)
	inst.NoInteract = true

	pkg := &core.ResolvedPackage{Name: pkgName, Version: pkgVer}
	pkgDir := paths.PackageVStoreDir(projectRoot, pkgName, pkgVer)
	sigPath := paths.PackageSigPath(pkgDir)

	index, err := casManager.GetIndex(pkgName, pkgVer)
	if err != nil || index == nil {
		return fmt.Errorf("package index not found in CAS for %s. It must be fetched first", pkgName)
	}

	if err := inst.VerifySignatureInternal(sigPath, pkg, index); err != nil {
		return fmt.Errorf("signature verification failed: %v", err)
	}

	utils.Info("Finalizing installation for %s@%s...", pkgName, pkgVer)
	inst.LinkFilesToDir(pkgDir, index)
	
	rootDest := filepath.Join(projectRoot, "node_modules", pkgName)
	utils.LinkDir(pkgDir, rootDest)
	
	utils.Success("Plugin %s@%s successfully linked to project.", pkgName, pkgVer)
	return nil
}
