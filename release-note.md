# XFPM - vG0.1.90

## [G0.1.90] - 2026-04-17

### Added

- **`xfpm audit fix`**:
  - Automatically resolve vulnerabilities by updating direct dependencies to their latest safe versions.
  - Interactive confirmation with clear security warnings about potential breaking changes.
  - **`--yes` / `-y` flag**: Support for automated/CI environments to skip interactive confirmation.
- **Enhanced Dependency Insights**:
  - The HTML report now displays a "Pulled in by" section for every vulnerability, showing the full recursive dependency path from the root project.
  - Eliminates the need for manual `xfpm list` calls to investigate vulnerability sources.

## [G0.1.88] - 2026-04-17

### Added

- **Premium Audit UI (XFPML)**:
  - **XyPriss Fast Markup Language (XFPML)**: Unified the security report template system into a dedicated `.xfpml` architecture.
  - **Optimized Dependency Graph**: Fully redesigned the D3.js force-directed layout with improved repulsion and collision physics.
  - **Enhanced Readability**: Graph labels now feature high-contrast light colors with a custom text-shadow.
  - **Interactive Viewport**: Added a new **Expand** feature for fullscreen graph exploration.

### Fixed

- Improved Mermaid.js configuration robustness (support for `maxTextSize` up to 1M).
- Fixed fragile `fmt.Sprintf` template rendering by switching to a robust `html/template` engine.

## [G0.1.87] - 2026-04-17

### Added

- **Enhanced `xfpm audit` UI**:
  - **Interactive Menu**: After scan completion, users can now choose between different visualizations.
  - **Dependency Tree View**: A new terminal-based visualization showing exactly how vulnerabilities are pulled into the dependency graph.
  - **Premium HTML Report**: Support for generating a high-fidelity standalone dashboard with Tailwind CSS and Mermaid.js graph visualizations.
  - **Direct Flags**: Support for `--tree` (`-t`) and `--html` (`-w`) to bypass interactive selection.

### Fixed

- Resolved non-constant format string warnings in `path_manager.go`.

## [G0.1.86] - 2026-04-17

### Added

- **Command `xfpm audit`**: Introduced a new Software Composition Analysis (SCA) tool that audits your project's dependency graph against the OSV (Open Source Vulnerabilities) database, identifying known security vulnerabilities.
- **Support for `peerDependenciesMeta`**: XFPM now correctly parses and respects the `peerDependenciesMeta` field in `package.json`, allowing peer dependencies to be marked as optional.

## [G0.1.85] - 2026-04-17

### Added

- **Multi-Package Listing**: Enhanced the `xfpm list` command to support multiple package arguments. Users can now check the location and dependents of several packages simultaneously (e.g., `xfpm list helmet express-rate-limit`).
- **Improved List Formatting**: Added clear visual separators between package details and optimized footer printing for multi-argument execution.

## [G0.1.84] - 2026-04-14

### Added

- **Command `xfpm create`**: Introduced a new command for project initialization via community generators. It behaves like `npm create` by automatically resolving `create-<name>` packages and executing them using the local JS runtime (`bun` or `npx`).

## [G0.1.83] - 2026-04-14

### Fixed

- **Global Path Consistency**: Unified global storage paths across all commands (`uninstall`, `list`, `run`). Fixed a bug where `xfpm rm -g` would look in the legacy `~/.xpm_global` directory instead of the new `~/.xpm/globals` structure.

## [G0.1.82] - 2026-04-10

### Added

- **Global Store Pruning & Migration**: The `xfpm store prune --legacy` command now scans for and migrates legacy global storage directories (e.g., `xpm_global_storage`, `xpm_store`) into the new unified CAS. This ensures a clean upgrade path for users coming from very old XFPM versions.

### Improved

- **Generalized Migration Engine**: Decoupled migration logic from project-local paths, allowing XFPM to safely migrate data from any identified legacy source.

# XFPM - vG0.1.81

## [G0.1.81] - 2026-04-10

### Added

- **Immediate Process PATH Injection**: XFPM now injects the global bin path into the current process environment. This allows post-install scripts and sub-commands to access newly installed binaries in the same session.
- **Windows Environment Broadcast**: On Windows, XFPM now broadcasts `WM_SETTINGCHANGE` after updating the PATH. This ensures that new terminals and Explorer pick up the change without a system restart.
- **Enhanced Profile Detection**: Improved logic for Bash, Zsh, and Fish profile discovery with prioritized ordering.

### Improved

- **User UX**: Automated "source" suggestions and profile path normalization.

# XFPM - vG0.1.80

## [G0.1.80] - 2026-04-10

### Added

- **Partition-Safe Script Execution**: XFPM now injects a localized `TMPDIR` within the package store during script execution. This prevents silent failures in `postinstall` scripts (like Bun or FileOnix) that attempt to move files from system temporary folders across different partitions.

### Fixed

- **Post-Install Reliability**: Resolved a significant regression where global tools would appear missing or broken because their installation scripts failed to persist downloaded binaries.

# XFPM - vG0.1.79

## [G0.1.79] - 2026-04-10

### Added

- **Premium PATH Management (Cross-Platform)**: XFPM now features a robust, automated environment setup. It intelligently discovers and configures your shell (Bash, Zsh, or Fish on Linux/macOS) and system environment (via PowerShell on Windows) to include `~/.xpm/bin`.
- **macOS Readiness**: Added specialized support for `.zprofile`, the default login shell for modern macOS.

### Improved

- **Code Modernization**: Optimized `path_manager.go` for clarity and modern Go standards.
- **Adaptive Binary Linking**: (Carried from G0.1.77) Ensures dynamic binaries (like Bun/Deno) are correctly discovered and linked.

# XFPM - vG0.1.78

## [G0.1.78] - 2026-04-10

### Added

- **Automatic PATH Management**: XFPM now automatically configures your system environment. During global installations, it detects your active shell (Bash or Zsh) and injects the necessary PATH entries into your profile (`.bashrc`, `.zshrc`, etc.) if they are missing. No manual configuration required!

### Fixed

- **Stable Link Lifecycle**: Refined the global installation hook to ensure PATH discovery happens seamlessly after all binary links are established.

# XFPM - vG0.1.77

## [G0.1.77] - 2026-04-10

### Added

- **Adaptive Binary Linking**: Switched from rigid metadata-based linking to an intelligent discovery algorithm. XFPM now correctly resolves binaries even if they have platform-specific extensions (like `.exe` appearing in NPM metadata on Linux) or if they are dynamically created by post-install scripts (fixing `bun` and `deno` global installs).
- **Execution Lifecycle Sync**: Corrected the order of operations to ensure all `postinstall` scripts finish executing before binaries are exported to the global path.

# XFPM - vG0.1.76

## [G0.1.76] - 2026-04-10

### Changed

- **UI Refinement**: Removed the automatic clearing of the terminal after installation. While intended for cleanliness, it proved disruptive to the visual flow and context. The installer now maintains a natural scrollback.

### Stable Fixes (carried from G0.1.75)

- **Unified Global Architecture**: Global installations remain in `~/.xpm/globals` with binaries in `~/.xpm/bin`.
- **Exhaustive Permission Management**: Recursive `chmod 755` for binary directories remains active to prevent `EACCES` issues.

# XFPM - vG0.1.75

## [G0.1.75] - 2026-04-10

### Added

- **UX - Clear Screen After Install**: As requested, the terminal is now automatically cleared after a successful installation, providing a clean slate for subsequent script logs.
- **Unified Global Architecture**: Global installations are now organized under `~/.xpm/globals` with binaries consistently exported to `~/.xpm/bin`. This eliminates "stale" binary links from previous experimental versions.

### Fixed

- **Exhaustive Permission Management**: Implemented recursive `chmod 755` for all files within a package's `bin/` directory. This decisively resolves `EACCES` errors on systems where reflink/hardlink metadata might not preserve executable bits.

# XFPM - vG0.1.74

## [G0.1.74] - 2026-04-10

### Fixed

- **EACCES Errors Resolved**: Fixed a critical bug where binary permissions were lost during Hardlink or Copy fallbacks. Executable bits are now strictly preserved and applied across all storage operations.
- **Robust Binary Linking**: Improved the reliability of binary symlinks in `.bin` directories to ensure they always point to the correct project-local or global-local virtual store paths.
- **Improved Bun Compatibility**: Refined the resolver and linking logic to better handle platform-specific optional dependencies required by Bun's install scripts.

# XFPM - vG0.1.73

## [G0.1.73] - 2026-04-09

### Added

- **"Ancestor Hoisting" Architecture**: Refactored the storage model to place the Virtual Store inside `node_modules/.xpm/vstore`. This ensures that Node/Bun's `realpath` resolution remains within the project boundary, allowing packages to naturally discover "phantom dependencies" in the root `node_modules`.
- **Zero-Copy Local Purity**: Every package is now reflinked (Copy-on-Write) from the Global CAS into the project-specific store. This provides 100% compatibility with native tools and write-heavy scripts without any extra disk space consumption.
- **Fixed Bun/Negotiator Bugs**: Resolved the critical issues where global links caused `require()` failures in Bun and other tools using realpath resolution.

# XFPM - vG0.1.72

## [G0.1.72] - 2026-04-09

### Added

- **Ultra-Global Hybrid Architecture**: Introduced a new storage model that balances extreme deduplication with 100% project compatibility.
- **Intelligent Shadowing**: Packages with install scripts or known local-write requirements (like **Prisma**) are automatically "shadowed" into `node_modules/.xpm/shadow` using **Reflinks (CoW)**. This ensures they are writable and project-isolated without consuming extra physical disk space.
- **NODE_PATH Injection**: Automatically injects `NODE_PATH` into script executions, allowing packages in the Global Virtual Store to resolve "phantom dependencies" located in the project root.
- **Near-Zero Footprint**: Native packages and standard libraries remain in the Global Virtual Store, making `node_modules` logically and physically ultra-light.

# XFPM - vG0.1.71

## [G0.1.71] - 2026-04-09

### Changed

- **Core**: Reverted the Virtual Store layout to be **Project-Local (`node_modules/.xpm/virtual_store`)** while maintaining the **Global CAS** for physical files. This restores 100% compatibility with "phantom dependencies" (hoisting) and tools requiring local write-access (like Prisma), while still providing machine-wide disk deduplication via Hardlinks/Reflinks.

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
