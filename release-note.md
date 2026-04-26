# XFPM - Release Notes

## [G0.1.165] - 2026-04-26

### Added

- **Interactive Privilege Authorization (TOFU)**: Redesigned the security enrollment engine. `xfpm plugin verify` now surfaces a beautiful, batched interactive UI across all pending plugins, presenting required System Privileges requested by the author for your explicit consent, before persisting them securely to configuration.
- **Maintainer Privilege Declaration**: Maintainers can now explicitly list essential hooks via the `xfpm.permissions` array in `package.json`, which are validated via fetching the master registry during signing (`xfpm sign`).
- **Configuration-Based Bypassing**: Introduced `xfpm.trustedPlugins` array. Declare trusted libraries in your project settings to securely dodge the interactive Trust-On-First-Use prompts in non-interactive CI lines securely.

### Improved

- **Targeted Lifecycle Execution**: Redesigned script extraction limits hook execution solely to explicitly requested packages during a targeted transaction (e.g., `xfpm install <pkg>`), vastly mitigating ambient root-level hook collateral clutter.
- **Robust Local Protocol**: Re-engineered resolution handling of the native `file:` protocol. The engine now reliably strips absolute prefixes and correctly parses local dependency nested `package.json` configurations without crashing SemVer evaluations.
- **Trailing Comma Resiliency**: The XHS JSONC payload configuration parser now flawlessly tolerates trailing commas syntax natively, increasing resilience against hand-written errors.
- **Centralized Pathing**: Refactored CLI path definitions out from root scope into a generalized `paths` module to reinforce modular integrity and DRY limits.

## [G0.1.160] - 2026-04-24

### Fixed

- **Legacy Home Migration**: Significantly improved `store prune --legacy` to handle full `~/.xpm` to `~/.xfpm` transitions, including binaries, globals, and storage merging.

## [G0.1.159] - 2026-04-24

### Added

- **Dynamic Release Messages**: The update check now displays a custom message from the registry if available, providing immediate context for new versions.

## [G0.1.158] - 2026-04-24

### Added

- **Self-Upgrade Command**: Introduced `xfpm upgrade` for manual, forced updates against the Nehonix GitHub registry.
- **Refined Updater**: Improved terminal feedback and increased timeouts for forced update checks.

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
