# XFPM - Release Notes

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
