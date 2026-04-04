package core

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/Nehonix-Team/xfpm-go/internal/utils"
)

type ResolvedPackage struct {
	Name                 string
	Version              string
	SemverVersion        *semver.Version
	Metadata             *VersionMetadata
	ResolvedDependencies map[string]string
	LocalPath            string // If set, this package is installed from a local path
}

type Platform struct {
	OS   string
	Arch string
	Libc string
}

func CurrentPlatform() Platform {
	os := runtime.GOOS
	switch os {
	case "darwin":
		// npm uses 'darwin'
	case "windows":
		os = "win32"
	}

	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		// keep as arm64
	}

	// Simple libc detection (heuristic)
	libc := "glibc"
	// In Go, detecting musl vs glibc is tricky without CGO or checking /lib
	return Platform{
		OS:   os,
		Arch: arch,
		Libc: libc,
	}
}

func (p Platform) matchesOS(pkgOS StringOrStringArray) bool {
	if len(pkgOS) == 0 {
		return true
	}
	for _, os := range pkgOS {
		if strings.HasPrefix(os, "!") {
			if os[1:] != p.OS {
				return true
			}
		} else if os == p.OS {
			return true
		}
	}
	return false
}

func (p Platform) matchesArch(pkgCPU StringOrStringArray) bool {
	if len(pkgCPU) == 0 {
		return true
	}
	for _, cpu := range pkgCPU {
		isNegated := strings.HasPrefix(cpu, "!")
		targetCPU := cpu
		if isNegated {
			targetCPU = cpu[1:]
		}

		match := (targetCPU == p.Arch) || 
		         (targetCPU == "x64" && p.Arch == "x86_64") || 
		         (targetCPU == "x86_64" && p.Arch == "x64") ||
		         (targetCPU == "arm64" && p.Arch == "aarch64") ||
		         (targetCPU == "aarch64" && p.Arch == "arm64")

		if isNegated {
			if !match {
				return true
			}
		} else if match {
			return true
		}
	}
	return false
}

func (p Platform) matchesLibc(pkgName string, pkgLibc StringOrStringArray) bool {
	if len(pkgLibc) == 0 {
		if strings.Contains(pkgName, "-musl") && p.Libc != "musl" {
			return false
		}
		if strings.Contains(pkgName, "-gnu") && p.Libc != "glibc" {
			return false
		}
		return p.Libc == "glibc" || p.OS != "linux"
	}
	for _, l := range pkgLibc {
		if l == p.Libc {
			return true
		}
	}
	return false
}

func (p Platform) IsCompatible(meta *VersionMetadata) bool {
	name := strings.ToLower(meta.Name)

	// OS and CPU are the primary filters. If they don't match, the package is rejected.
	if !p.matchesOS(meta.OS) || !p.matchesArch(meta.CPU) {
		return false
	}

	// Libc is more of a heuristic in many packages
	if !p.matchesLibc(name, meta.Libc) {
		return false
	}

	// Name-based safety filters for Linux
	if p.OS == "linux" {
		// Only filter by name if the package DOES NOT specify OS or CPU (generic packages)
		// because if it specify them and they match (checked above), we should trust it.
		if len(meta.OS) == 0 && len(meta.CPU) == 0 {
			if strings.Contains(name, "android") || strings.Contains(name, "darwin") || strings.Contains(name, "win32") {
				return false
			}
		}

		if p.Libc == "glibc" && strings.Contains(name, "-musl") {
			return false
		}
		if p.Libc == "musl" && (strings.Contains(name, "-gnu") || strings.Contains(name, "-glibc")) {
			return false
		}
	}

	return true
}

type job struct {
	name       string
	req        string
	isOptional bool
}

type Resolver struct {
	registry        *RegistryClient
	cas             *Cas
	resolved        sync.Map          // string -> *ResolvedPackage
	resolutionCache sync.Map          // string -> string (name@req -> version)
	visited         sync.Map          // string -> bool
	platform        Platform
	concurrency     int
	overrides       map[string]string
	onProgress      func(string)
	Update          bool            // Global update mode
	ForcePackages   map[string]bool // Packages to force resolve (ignore cache)
	LocalPackages   map[string]string // Package name -> local absolute path
	Redirects       map[string]string // Package name -> Redirect Target
	redirectsMu     sync.Mutex
}


func (r *Resolver) SetLocalPackages(local map[string]string) {
	r.LocalPackages = local
}

func (r *Resolver) SetOnProgress(fn func(string)) {
	r.onProgress = fn
}

func NewResolver(registry *RegistryClient, cas *Cas) *Resolver {
	return &Resolver{
		registry:    registry,
		cas:         cas,
		platform:    CurrentPlatform(),
		concurrency: 128,
		overrides:   make(map[string]string),
		Redirects:   make(map[string]string),
	}
}

func (r *Resolver) SetOverrides(overrides map[string]string) {
	r.overrides = overrides
}

func (r *Resolver) ResolveTree(ctx context.Context, rootDeps map[string]string) ([]*ResolvedPackage, map[string]string, error) {
	queue := make(chan job, 10000)
	results := make(chan error, 10000)
	
	active := 0
	var mu sync.Mutex
	
	for name, req := range rootDeps {
		key := name + "@" + req
		if _, loaded := r.visited.LoadOrStore(key, true); !loaded {
			queue <- job{name: name, req: req}
			active++
		}
	}

	if active == 0 {
		return nil, nil, nil
	}

	for i := 0; i < r.concurrency; i++ {
		go func() {
			for j := range queue {
				err := r.resolvePackage(ctx, j.name, j.req, j.isOptional, queue, &active, &mu)
				results <- err
			}
		}()
	}

	for active > 0 {
		select {
		case err := <-results:
			mu.Lock()
			active--
			if err != nil {
				mu.Unlock()
				return nil, nil, err
			}
			if active == 0 {
				close(queue)
			}
			mu.Unlock()
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	var all []*ResolvedPackage
	r.resolved.Range(func(key, value interface{}) bool {
		all = append(all, value.(*ResolvedPackage))
		return true
	})

	// Circular dependency resolution Pass
	for _, pkg := range all {
		resolvedDeps := make(map[string]string)
		
		checkDeps := func(deps map[string]string) {
			for dName, dReq := range deps {
				if v, ok := r.findCompatibleVersion(dName, dReq); ok {
					resolvedDeps[dName] = v
				}
			}
		}

		checkDeps(pkg.Metadata.Dependencies)
		checkDeps(pkg.Metadata.OptionalDependencies)
		checkDeps(pkg.Metadata.PeerDependencies)

		pkg.ResolvedDependencies = resolvedDeps
	}

	rootVersions := make(map[string]string)
	for name, req := range rootDeps {
		realName, realReq := r.parseAlias(name, req)
		
		r.redirectsMu.Lock()
		for {
			if dest, ok := r.Redirects[realName]; ok {
				realName = dest
				realReq = "latest"
			} else {
				break
			}
		}
		r.redirectsMu.Unlock()

		cacheKey := realName + "@" + realReq
		if v, ok := r.resolutionCache.Load(cacheKey); ok {
			rootVersions[name] = v.(string)
		} else {
			r.resolved.Range(func(key, val interface{}) bool {
				pkg := val.(*ResolvedPackage)
				if pkg.Name == realName {
					rootVersions[name] = pkg.Version
					return false
				}
				return true
			})
		}
	}

	return all, rootVersions, nil
}

func (r *Resolver) findCompatibleVersion(name, req string) (string, bool) {
	// 1. Check if we have a version satisfying this in the resolved set
	var best *semver.Version
	var bestStr string

	constraint, err := semver.NewConstraint(req)
	if err != nil {
		// If req is not a valid semver (like a tag), we might have it exactly
		versionKey := name + "@" + req
		if _, ok := r.resolved.Load(versionKey); ok {
			return req, true
		}
		return "", false
	}

	r.resolved.Range(func(key, value interface{}) bool {
		pkg := value.(*ResolvedPackage)
		if pkg.Name == name {
			if constraint.Check(pkg.SemverVersion) {
				if best == nil || pkg.SemverVersion.GreaterThan(best) {
					best = pkg.SemverVersion
					bestStr = pkg.Version
				}
			}
		}
		return true
	})

	if best != nil {
		return bestStr, true
	}
	return "", false
}

func (r *Resolver) resolvePackage(ctx context.Context, name, req string, isOptional bool, queue chan job, active *int, mu *sync.Mutex) error {
	realName, realReq := r.parseAlias(name, req)
	if r.onProgress != nil {
		r.onProgress(realName)
	}

	// Check resolution cache
	cacheKey := realName + "@" + realReq
	shouldForce := r.Update || (r.ForcePackages != nil && r.ForcePackages[realName])

	if !shouldForce {
		if val, ok := r.resolutionCache.Load(cacheKey); ok {
			// Already resolving/resolved this specific constraint
			version := val.(string)
			versionKey := realName + "@" + version
			if _, loaded := r.resolved.Load(versionKey); loaded {
				return nil
			}
		}
	} else {
		utils.Log("FORCED", fmt.Sprintf("Fresh resolution for %s@%s", realName, realReq))
	}

	var pkgInfo *RegistryPackage
	var err error

	if path, ok := r.LocalPackages[realName]; ok {
		// Local package: load from disk
		pkgJson, lerr := LoadPackageJson(filepath.Join(path, "package.json"))
		if lerr != nil {
			return fmt.Errorf("loading local package.json for %s: %w", realName, lerr)
		}
		pkgInfo = &RegistryPackage{
			Name:     pkgJson.Name,
			DistTags: map[string]string{"latest": pkgJson.Version},
			Versions: map[string]VersionMetadata{
				pkgJson.Version: {
					Name:    pkgJson.Name,
					Version: pkgJson.Version,
					Dependencies: pkgJson.Dependencies,
					OptionalDependencies: pkgJson.OptionalDependencies,
					PeerDependencies: pkgJson.PeerDependencies,
					Xfpm: pkgJson.Xfpm,
					// Bin and other fields will be handled by installer if needed
				},
			},
		}
	} else {
		pkgInfo, err = r.registry.FetchPackage(ctx, realName, shouldForce)
		if err != nil {
			if isOptional {
				return nil
			}
			return err
		}
	}

	version, err := r.bestMatch(pkgInfo, realReq)
	if err != nil {
		if !shouldForce {
			// Cache miss for this version, maybe newly published? Fetch fresh!
			pkgInfo, err = r.registry.FetchPackage(ctx, realName, true)
			if err == nil {
				version, err = r.bestMatch(pkgInfo, realReq)
			}
		}
		
		if err != nil {
			if isOptional {
				return nil
			}
			return err
		}
	}

	if shouldForce {
		utils.Log("UPDATE", fmt.Sprintf("%s resolved to %s", realName, version))
	}

	r.resolutionCache.Store(cacheKey, version)
	versionKey := realName + "@" + version
	if _, loaded := r.resolved.LoadOrStore(versionKey, &ResolvedPackage{}); loaded {
		// Someone else is already handling this version or it's done
		return nil
	}

	meta := pkgInfo.Versions[version]

	if msg, ok := meta.Xfpm["message"]; ok {
		utils.Warn("Package '%s': %s", realName, msg)
	}

	if redirect, ok := meta.Xfpm["redirect"]; ok {
		utils.Warn("Package '%s' redirects to '%s'. Resolving new target...", realName, redirect)
		
		r.redirectsMu.Lock()
		r.Redirects[realName] = redirect
		r.redirectsMu.Unlock()

		mu.Lock()
		*active++
		queue <- job{name: redirect, req: "latest", isOptional: isOptional}
		
		if r.overrides == nil {
			r.overrides = make(map[string]string)
		}
		r.overrides[realName] = "npm:" + redirect
		mu.Unlock()

		r.resolved.Delete(versionKey)
		return nil
	}

	if !r.platform.IsCompatible(&meta) {
		// Silently skipped
		r.resolved.Delete(versionKey)
		return nil
	}

	sv, _ := semver.NewVersion(version)
	resolved := &ResolvedPackage{
		Name:                 realName,
		Version:              version,
		SemverVersion:        sv,
		Metadata:             &meta,
		ResolvedDependencies: make(map[string]string),
	}
	r.resolved.Store(versionKey, resolved)

	// Add dependencies to queue
	mu.Lock()
	for dName, dReq := range meta.Dependencies {
		dKey := dName + "@" + dReq
		if _, loaded := r.visited.LoadOrStore(dKey, true); !loaded {
			*active++
			queue <- job{name: dName, req: dReq, isOptional: false}
		}
	}
	for dName, dReq := range meta.OptionalDependencies {
		dKey := dName + "@" + dReq
		if _, loaded := r.visited.LoadOrStore(dKey, true); !loaded {
			*active++
			queue <- job{name: dName, req: dReq, isOptional: true}
		}
	}
	for dName, dReq := range meta.PeerDependencies {
		dKey := dName + "@" + dReq
		if _, loaded := r.visited.LoadOrStore(dKey, true); !loaded {
			*active++
			queue <- job{name: dName, req: dReq, isOptional: true} // Peers are implicitly optional for us
		}
	}
	mu.Unlock()

	return nil
}

func (r *Resolver) parseAlias(name, req string) (string, string) {
	currentName := name
	currentReq := req

	// 1. Overrides
	if val, ok := r.overrides[name]; ok {
		currentReq = val
	}

	// 2. npm: aliases
	if strings.HasPrefix(currentReq, "npm:") {
		stripped := currentReq[4:]
		atIdx := strings.LastIndex(stripped, "@")
		if atIdx > 0 {
			currentName = stripped[:atIdx]
			currentReq = stripped[atIdx+1:]
		} else {
			currentName = stripped
			currentReq = "latest"
		}
	}

	// 3. workspace: (normalize to latest for now as we don't handle monorepos yet)
	if strings.HasPrefix(currentReq, "workspace:") {
		currentReq = "latest"
	}

	return currentName, currentReq
}

func (r *Resolver) bestMatch(pkg *RegistryPackage, req string) (string, error) {
	if req == "latest" || req == "*" || req == "" {
		if v, ok := pkg.DistTags["latest"]; ok {
			return v, nil
		}
	}

	if _, ok := pkg.Versions[req]; ok {
		return req, nil
	}

	constraint, err := semver.NewConstraint(req)
	if err != nil {
		// Check tags if not a valid semver constraint
		if v, ok := pkg.DistTags[req]; ok {
			return v, nil
		}
		return "", fmt.Errorf("invalid semver constraint %s for %s", req, pkg.Name)
	}

	var best *semver.Version
	for vStr := range pkg.Versions {
		v, err := semver.NewVersion(vStr)
		if err != nil {
			continue
		}
		if constraint.Check(v) {
			if best == nil || v.GreaterThan(best) {
				best = v
			}
		}
	}

	if best != nil {
		return best.Original(), nil
	}

	return "", fmt.Errorf("no version found for %s satisfying %s", pkg.Name, req)
}

