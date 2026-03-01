# XFPM Release Notes - Version G0.1.42

## Overview

Version G0.1.42 introduces critical updates to the package resolution engine, cross-platform compatibility layers, and the official NPM deployment wrapper. This release focuses on reliability in non-interactive environments and improved dependency tree consistency.

## Major Changes

### NPM Deployment Wrapper

- Implementation of the `xypriss-cli` package on the NPM registry.
- Integration of a post-install trigger that executes the native deployment sequence.
- Added comprehensive documentation regarding administrative privilege requirements for global installations.

### Installation Infrastructure

- Enhanced Unix and Windows installers with automated detection of existing XyPriss components.
- Implementation of an explicit cleanup sequence prior to re-installation.
- Added TTY detection to handle piped and non-interactive sessions (e.g., CI/CD or automated scripts).
- Reverted delivery endpoints to the private Nehonix DLL infrastructure for secure script sourcing.

### Core Engine and Resolver

- Updated the dependency resolver to support flexible architecture naming conventions (reconciling x64/x86_64 and arm64/aarch64).
- Implementation of recursive force extraction: the `--force` flag now applies to the entire dependency tree to ensure store integrity.
- Integrated explicit skip logging in the resolver to provide transparency regarding platform-specific package exclusions.
- Synchronized CLI version output with the internal package metadata.

### Legal and Compliance

- Standardized Nehonix OSL (NOSL) license headers across all core CLI implementation files.
- Updated copyright notices to current requirements.

## Technical Improvements

- Refined Linux-specific naming heuristics in the compatibility layer.
- Optimized build script support for multi-architecture compression.
- Improved error handling during binary stream synchronization.

---

**Build Metadata**

- Reference Commit: c91b771
- Internal Version: G0.1.42
- Publisher: Nehonix
