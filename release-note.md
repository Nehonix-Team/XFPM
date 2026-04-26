# XFPM - Release Notes

## [G0.1.174] - 2026-04-26

### Fixed

- **CLI Status Accuracy**: Fixed a bug where a plugin lacking a local installation or signature was incorrectly marked as `VERIFIED` if a config entry existed. It now properly reports as `NOT_INSTALLED` or `KEY_MISMATCH`.
- **UI Toggle State**: Fixed Web UI to accurately leave the `Trust` toggle unchecked for plugins that are still in the `PENDING` state, even in Review Mode.

## [G0.1.173] - 2026-04-26

### Fixed

- **Robust Detection Fallback**: Implemented direct tarball inspection as a fallback when registry metadata lacks the `Files` array. This ensures all XyPriss plugins (like `xypriss-swagger`) are correctly identified.
- **Badge Consistency**: Fixed a major UI bug where all plugins in review mode were incorrectly badged as "Authorized". Status badges now accurately reflect the project configuration state.

## [G0.1.172] - 2026-04-26

### Fixed

- **Verify Shorthand Fix**: Restored missing `-w` and `--html` flags in `xfpm plugin verify` that were accidentally removed during modularization.
- **Dependency Scan Fix**: Fixed `plugin list` to correctly scan both `dependencies` and `devDependencies`, ensuring all plugins are identified.

### Added

- **Interactive Revocation**: The Permission Review Dashboard (`-r`) now features a "Revoke" button to instantly retire trust and permissions for any plugin directly from the UI.

## [G0.1.171] - 2026-04-26

### Added

- **Plugin Review Mode**: Added `--review` (or `-r`) flag to `xfpm plugin list` to open the web dashboard for all project plugins, allowing for easy permission updates.
- **Robust Plugin Detection**: Fixed a bug in `plugin list` where it failed to identify plugins; it now correctly scans dependencies using both local and registry metadata.
- **Local Scan Flag**: Added `--local` (or `-l`) flag to `plugin list` for offline-only dependency scans.

## [G0.1.170] - 2026-04-26

### Added

- **Plugin Revoke Enhancements**: Revoking trust now automatically re-queues plugins into the pending list for re-verification. Added `--no-pending` flag for clean revocation.
- **Ignore Pattern Fix**: Corrected `.gitignore` logic that was incorrectly shadowing CLI source files.

## [G0.1.169] - 2026-04-26

### Added

- **Plugin Modularization**: Refactored the core plugin management engine into a standalone internal module for better G3 architecture compliance and maintainability.
- **UI Aesthetic Overhaul**: Significant improvements to the Verification Dashboard UI—increased typography scale, improved spacing, and enhanced scalability for high-density permission reviews.

## [G0.1.168] - 2026-04-26

### Added

- **Plugin Verification Dashboard**: Massive UI overhaul. Added a beautiful, interactive web-based dashboard (`--html` or `-w`) for granular project plugin authorization.
- **Interactive Terminal UI**: Fallback verification now uses an interactive multi-select menu for better UX.
- **Shared Utilities**: Centralized `OpenBrowser` logic into a shared internal utility for consistent cross-platform behavior.

## [G0.1.167] - 2026-04-26

### Added

- **Permissions Hardening**: Added robust duplication validation inside `xfpm sign` and a built-in `--fix` flag capable of auto-correcting any duplicates inside `package.json` dynamically.

## [G0.1.166] - 2026-04-26

### Fixed

- **Runtime Path Resolution**: Fixed a severe logic regression in `EnsureRuntime` where `exec.LookPath` incorrectly triggered unmanaged installation warnings by detecting the local `node_modules/.bin/bun` symlink injected into PATH by `xfpm run`, shadowing the global `~/.xfpm/bin` installation. The engine now prioritizes exact explicit global checking.
