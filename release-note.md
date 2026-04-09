# XFPM - vG0.1.70

## [G0.1.70] - 2026-04-09

### Fixed

- **Core**: Resolved a critical regression where **post-install scripts failed** in the Global Virtual Store (e.g. `zstd-napi`, `bun`). The root cause was an off-by-one error in relative symlink depth calculation, preventing `node` from reaching sibling dependencies.

# XFPM - vG0.1.69

## [G0.1.69] - 2026-04-09

### Added

- **Core**: Transitioned to **Global Virtual Store (`~/.xpm/virtual_store`)**. Package layouts are now shared machine-wide, reducing repetitive extraction and linking work.
- **Deduplication**: Achieved **Ultra-Light node_modules** (measured at < 50KB for standard projects). Project directories now contain only symlinks to the shared global store.
- **Offline Support**: Implemented **Local-First Resolution**. XFPM now prioritizes locally cached metadata and CAS indices, allowing near-instant installations without network access for already downloaded packages.
- **Performance**: Improved dependency resolution speed by skipping network roundtrips for known versions.

# XFPM - vG0.1.68

## [G0.1.68] - 2026-04-09

### Added

- **Core**: Introduced a **Global Content-Addressable Storage (CAS)** system. Packages are now stored centrally in `~/.xpm/storage`, allowing all projects to share identical physical files and drastically reducing disk space usage.
- **Core**: Implemented native **Reflink (Copy-on-Write)** support for Linux (XFS, Btrfs, APFS) using `ioctl(FICLONE)`, providing instant, space-efficient file cloning with full isolation.
- **Migration**: Introduced an intelligent **Legacy Migration System**. XFPM now automatically detects storage from older versions and offers to migrate it to the global store.
- **CLI**: Added the `xfpm store prune --legacy [path]` command for high-performance recursive cleanup of old project-local storage across the machine.
- **Documentation**: Added comprehensive architecture documentation for storage optimization in `architecture/storage_optimization.en.md`.

### Improved

- **Core**: Optimized file linking with a tiered strategy (Reflink -> Hardlink -> Copy).
- **Security**: CAS storage is now strictly **Read-Only** (0444/0555) to protect global store integrity against modification by package scripts.
- **CLI**: The storage path can now be overridden via the `XFPM_STORAGE` environment variable.

# XFPM - vG0.1.67

## [G0.1.67] - 2026-04-08

### Added

- **CLI**: Implemented the `xfpm publish` command wrapping `npm publish` natively with flag-forwarding to simplify deployments while maintaining visual consistency.
- **CLI**: Introduced the `--save-peer` (`-R`) flag to correctly inject installations directly into a project's `peerDependencies`.
- **UI**: Added real-time network speed tracking (e.g. `[15 MB/s]`) seamlessly integrated into the extraction progress ticker.

### Improved

- **Core**: Hardened local package installation resolving (`xfpm i -fP <chemin>`). When installing from a local directory, XFPM now parses the target's `package.json` to intelligently restrict installation to properties defined in the `"files"` array, respecting strict packaging semantics exactly like a native `.tgz` installation.

# XFPM - vG0.1.66

## [G0.1.66] - 2026-04-05

### Changed

- **System**: Internal updates and preparation for architecture enhancements.

# XFPM - vG0.1.65

### Fixed

- **Core**: Resolved a synchronization issue where redirected packages (e.g. `nquickdev` -> `fileonix`) were downloaded correctly but failed to replace the legacy package name in the project's `package.json` when running `xfpm install` without specific arguments.

# XFPM - vG0.1.64

## [G0.1.64] - 2026-04-05

### Added

- **CLI**: Implemented the `xfpm dev` alias which automatically proxies execution to `xfpm run dev`.
- **CLI**: Added support for the `--global` (or `-g`) flag on the `xfpm update` command.

### Fixed

- **Core**: Enhanced the registry redirection logic. When a legacy package redirects to a new target during installation, XFPM now cleanly strips the legacy package string from the user's `package.json` arrays, ensuring only the new target remains.

## [G0.1.63] - 2026-04-04

### Changed

- **Registry**: Restructured the `"xfpm"` redirection payload. The explicit target resolution rule now enforces a robust object architecture formatting: `"xfpm": { "redirect": { "target": "new-package-name", "message": "reason" } }`.

## [G0.1.62] - 2026-04-04

### Added

- **Registry**: Package maintainers can now define native package redirections inside their `package.json` utilizing the `"xfpm"` object (e.g., `"xfpm": { "redirect": "new-lib", "message": "Use new-lib instead" }`). Upon installation, XFPM will automatically intercept the deprecated package, display the informative message, and seamlessly redirect the installation to the designated replacement. The consuming project's `package.json` is updated securely to reflect the new target.
- **CLI**: The `xfpm exec` command fully integrates transparent flag-forwarding (e.g. `--force`), passing flags explicitly to executed binaries.

## [G0.1.61] - 2026-

04-02

### Fixed

- **Core**: Resolved a critical structural bug where `xfpm update` incorrectly stripped dependency boundaries (such as `peerDependencies`, `devDependencies`, and `optionalDependencies`) and merged them unconditionally into standard `dependencies` when specific packages were updated via CLI (e.g., `xfpm update xypriss`). XFPM now correctly memorizes and restores the package to its original structural block unless explicitly instructed otherwise via flags (`-D`, `-O`).

## [G0.1.60] - 2026-04-02

### Fixed

- **Core**: Resolved a critical oversight where `peerDependencies` in the root `package.json` were ignored during standard installation. XFPM now correctly collects, resolves, and installs these dependencies, ensuring full compatibility with plugins and libraries that rely on peer-level isolation.

### Improved

- **Metadata**: Refactored internal dependency aggregation logic in `PackageJson.AllDependencies` for better accuracy during project analysis.

## [G0.1.59] - 2026-03-28

### Fixed

- **Core**: Fixed a critical bug where `xfpm install <pkg>` (or `xfpm i <pkg>`) would fail to create a `package.json` file in a directory that did not already have one. XFPM now correctly initializes a default `package.json` with the current directory name and version `1.0.0` when installing packages for the first time.

### Improved

- **DX**: Streamlined first-time initialization experience, making it easier to start new projects by directly adding dependencies.
