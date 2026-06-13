package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Lockfile struct {
	LockfileVersion int                      `json:"lockfileVersion"`
	Name         string                   `json:"name"`
	Version      string                   `json:"version"`
	Dependencies map[string]LockfileEntry `json:"dependencies"`
	RootVersions map[string]string        `json:"root_versions"`
}

type LockfileEntry struct {
	Version              string            `json:"version"`
	Integrity            string            `json:"integrity"`
	Tarball              string            `json:"tarball"`
	ResolvedDependencies map[string]string `json:"resolved_dependencies"`
	DependencyRealNames  map[string]string `json:"dependency_real_names,omitempty"`
	Bin                  json.RawMessage   `json:"bin,omitempty"`
}

// ReadLockfile reads and parses the xfpm.resolve.lock file from the project root.
func ReadLockfile(projectRoot string) (*Lockfile, error) {
	lockPath := filepath.Join(projectRoot, "xfpm.resolve.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}

	var lockfile Lockfile
	if err := json.Unmarshal(data, &lockfile); err != nil {
		return nil, err
	}

	if lockfile.LockfileVersion < 2 {
		return nil, fmt.Errorf("outdated lockfile version, requires regeneration")
	}

	return &lockfile, nil
}

// WriteLockfile generates and writes the xfpm.resolve.lock file based on resolved packages.
func WriteLockfile(projectRoot string, pkgName string, pkgVersion string, resolvedPackages []*ResolvedPackage, rootVersions map[string]string) error {
	lockfile := Lockfile{
		LockfileVersion: 2,
		Name:            pkgName,
		Version:         pkgVersion,
		Dependencies:    make(map[string]LockfileEntry),
		RootVersions:    rootVersions,
	}

	for _, pkg := range resolvedPackages {
		key := pkg.Name + "@" + pkg.Version
		
		integrity := ""
		tarball := ""
		if pkg.Metadata != nil {
			integrity = pkg.Metadata.Dist.Integrity
			if integrity == "" {
				integrity = pkg.Metadata.Dist.Shasum
			}
			tarball = pkg.Metadata.Dist.Tarball
		}

		entry := LockfileEntry{
			Version:              pkg.Version,
			Integrity:            integrity,
			Tarball:              tarball,
			ResolvedDependencies: pkg.ResolvedDependencies,
			DependencyRealNames:  pkg.DependencyRealNames,
			Bin:                  pkg.Metadata.Bin,
		}

		lockfile.Dependencies[key] = entry
	}

	data, err := json.MarshalIndent(lockfile, "", "  ")
	if err != nil {
		return err
	}

	lockPath := filepath.Join(projectRoot, "xfpm.resolve.lock")
	return os.WriteFile(lockPath, data, 0644)
}
