# XFPM - Release Notes

## [G0.1.157] - 2026-04-24

### Fixed

- **Bun Runtime Resolution**: Corrected the package identifier from `@oven/bun` to `bun` in the runtime bootstrapper, resolving 404 errors during auto-installation.

## [G0.1.156] - 2026-04-24

### Improved

- **Hardened Runtime Verification**: Refactored `EnsureRuntime` to use robust system-wide detection via `exec.LookPath`.
- **Managed Runtime Prompt**: Added an intelligent prompt to detect and offer managed reinstallation of "shadow" Bun instances (binaries found outside the XFPM environment), ensuring ecosystem consistency.
- **Ecosystem Sync**: Updated core libraries for better alignment with the latest XHSC Hyper-System features.

## [G0.1.155] - 2026-04-24

### Added

- **Modular CLI Architecture**: Completely refactored the `xfpm` entry point into a dedicated `cmd` package. Subcommands are now isolated in their own modules, improving maintainability and scalability.
- **Centralized Path Management**: Introduced a unified `internal/paths` module to manage all XFPM system paths. This ensures cross-platform consistency and eliminates hardcoded paths in the core engine.
- **Improved UX - Path Standarization**: Renamed the base system directory from `.xpm` to `.xfpm` for consistency with the CLI name.
- **Automatic Home Migration**: Implemented a seamless migration logic that automatically renames legacy `~/.xpm` directories to the new `~/.xfpm` standard on the first run.
- **Fix: Internal Directory Leaks**: Hardened the CAS engine to prevent internal XFPM folders (`files`, `indices`, etc.) from being created in the project root by defaulting to global storage.

### Changed

- **Internal Structure**: All CLI command definitions have been moved from `main.go` and various root files to `cmd/xfpm/cmd/`.

## [G0.1.152] - 2026-04-24

### Added

- **XFPM Runtime Orchestrator**: Automatic detection and installation of Bun runtime using the internal XFPM engine.
- **Command**: `xfpm runtime rm bun` for easy runtime uninstallation.
- **DRY Refactoring**: Bootstrapper now uses the core installer/resolver stack for @oven/bun.

# XFPM - vG0.1.151
