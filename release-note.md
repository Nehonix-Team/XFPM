## [G0.1.197] - 2026-05-02

### CAS Integrity & Extraction Hardening

- **Zero-Byte File Guard**: Fixed a critical bug in the Content Addressable Storage (CAS) where interrupted extractions could leave 0-byte files in the cache. XFPM now strictly validates file sizes during the "ensure extracted" phase, forcing a re-download if a file is unexpectedly empty.
- **Professional Log Aesthetics**: Removed all emojis from CLI outputs and terminal logs in compliance with Nehonix professional standards. Replaced with structured ANSI color-coded prefixes (`[INFO]`, `[SUCCESS]`, `[ERROR]`).
- **Improved Concurrency Safety**: Hardened the streaming extractor and linking pool to better handle race conditions during high-concurrency global installations.

## [G0.1.196] - 2026-05-02

### Orchestration Cleanup & Engine Hardening

- **Robust Orphan Cleaning**: Implemented a global regex-based cleanup pass in the orchestration engine. This ensures that any `// xfpm:` markers from unselected features are cleanly removed from the final project source code.
- **Enhanced Injection Logic**: Refactored `InjectCode` and added `CleanOrphans` to the XRU engine to improve reliability and maintain code cleanliness in generated projects.
- **XRU Standalone Migration**: Extracted XRU (XyPriss Rule Unit) into its own repository (`tools/xru`) for better reusability across the ecosystem.
- **Universal Injection Syntax**: Introduced `@*INJECT` (e.g., `@GOINJECT`) with automatic language filtering for global rules.
- **New XRU Operations**: Added `&append:` for array manipulation and `&regex:` for pattern-based text transformation.
- **New Command**: Added `xfpm xru` to allow developers to manually apply rule files to their projects.

## [G0.1.195] - 2026-05-02

### Modular XRU Orchestration & STP Engine

- **Modularized XRU Engine**: Extracted orchestration logic into a dedicated `internal/xru` package, separating AST definitions, token-based parsing, and transformation execution.
- **Structured Text Patcher (STP)**: Implemented a non-destructive transformation engine that operates on raw text using brace-depth tracking. This ensures that comments, custom indentation, and XyPriss-specific variable syntax (`&(pkg).name`) are preserved during patching.
- **Native XRU Parser**: Replaced restrictive JSON-based rule parsing with a native DSL parser that supports unquoted keys, trailing commas, and native comments within `.xru` files.
- **Recursive Removal Logic**: Enhanced the `&rm` action to intelligently remove entire structured blocks (objects/arrays) or individual keys while maintaining valid file structure.
- **Official XRU Documentation**: Added a comprehensive [README.md](file:///home/idevo/Documents/projects/XyPriss/tools/xfpm-go/internal/xru/README.md) for the XRU syntax.

## [G0.1.194] - 2026-04-30

### Sudo Resilience & UX Hardening

- **Sudo-Aware Path Resolution**: Implemented an intelligent home directory detector in the paths module. XFPM now correctly identifies the original developer's home directory via `SUDO_USER` environment variables even when running with `sudo`. This ensures that global tools (like Bun) and configurations are consistently found across different privilege levels, eliminating redundant installation prompts.
- **Annotation-Based Runtime Enforcement**: Refactored the automated JS runtime check to use a declarative system. Commands now explicitly state if they require the JS environment via Cobra annotations. 
- **Streamlined Administrative Workflows**: Administrative and informational commands (`upgrade`, `help`, `version`, `sign`) no longer trigger `EnsureRuntime` checks. This significantly speeds up binary maintenance and eliminates the confusing "Would you like to install Bun?" prompt when performing system-level upgrades as root.

## [G0.1.193] - 2026-04-30

### Security & Developer Experience (DX)

- **Deep Scan Permission Discovery**: Introduced `Plugin.inspect()` in the Plugin API. This utility performs a deep runtime scan of plugin instances, mapping implemented hooks and properties (like `globalMiddleware`) to their official cryptographic permission IDs. This eliminates guesswork for plugin authors when preparing their security manifests.
- **Mandatory Config Auto-Fix**: The `xfpm sign` command now enforces the presence and correctness of the `xypriss.config.jsonc` security contract. 
- **Intelligent Signing Logic**: When the `--fix` flag is passed, XFPM will automatically:
    - Create a missing `xypriss.config.jsonc` with mandatory metadata.
    - Inject missing `$internal` plugin type declarations into existing configs.
    - De-duplicate entries in the `xfpm.permissions` array.
- **Centralized Path Logic**: Refactored the signing and identity management modules to use a unified path resolution system, improving cross-platform reliability.

## [G0.1.192] - 2026-04-30

### Stability & Bugfixes

- **Cross-Platform Execution Guard**: Resolved a major issue on Windows where native `.exe` binaries (such as the Bun runtime) were linked into `node_modules/.bin` without their `.exe` extension. This prevented `EnsureRuntime` and the OS from locating and executing the binaries. XFPM now intelligently detects native Windows executables during the adaptive linking phase and appends the `.exe` extension automatically.

## [G0.1.191] - 2026-04-30

### Mandatory Security Disclosure

- **Mandatory Permissions Field**: Enforced strict disclosure of plugin permissions. The `xfpm.permissions` field is now mandatory in `package.json` for the `sign` command. Plugins failing to disclose their required permissions (even if empty `[]`) will be rejected.

## [G0.1.190] - 2026-04-29

### Lifecycle & Cross-Platform Optimization

- **Native Build Tool Resolution**: Fixed a critical bug where lifecycle scripts (e.g., `argon2` install) failed with "node-gyp-build: not found" by correctly injecting `node_modules/.bin` into the execution `PATH`.
- **Win32 Native Junctions**: Replaced the slow `cmd /c mklink /j` process spawning with native Windows API calls, providing near-instant directory linking on Windows systems.
- **Optimized Concurrency**: Tuned Windows installation parallelism to avoid disk contention and antivirus-related bottlenecks during high-speed extractions.

## [G0.1.189] - 2026-04-28

### CLI & Security Hardening

- **New `start` Command**: Added `xfpm start` as a direct alias for `xfpm run start`, streamlining the development-to-production workflow.
- **Strict Signature Validation**: Enforced strict existence checks for all files listed in `package.json` during the signing process. Plugins missing mandatory files will now fail to sign unless bypassed with `--`.

## [G0.1.188] - 2026-04-27

### Deep Discovery & Sync

- **Fluid Discovery Restart**: Integrated a deep re-scanning mechanism that mimics fresh CLI execution, ensuring all project changes are captured without manual restarts.
- **Real-Time Progress**: Added visual progress tracking for deep discovery cycles with high-performance dashboard synchronization.
- **Signal Hardening**: Resolved critical signal conflicts between `SignalManager` and UI handlers, ensuring graceful termination and terminal state restoration.

### UI/UX Enhancements

- **Batch Trust Authorization**: Users can now authorize multiple plugins simultaneously via the new "Trust Selected" action bar integration.
- **High-Tech Overlay**: Implemented an automated discovery overlay for installations, providing seamless visual feedback during background metadata refreshes.
- **Advanced Revoke Modal**: Overhauled revocation flow with interactive choice between re-queuing and physical uninstallation.

### Stability & Bugfixes

- **State Synchronization**: Fixed race conditions in configuration persistence and ensured total in-memory state consistency after UI actions.
- **SSE Reliability**: Restored missing event listeners for real-time synchronization and session lifecycle management.
- **Path Standardization**: Unified all filesystem operations using the internal paths module for cross-platform reliability.

## [G0.1.176] - 2026-04-26

- **Pending Plugins UI**: Plugins flagged as `PENDING` in the review dashboard now default to a transparent and desaturated grey aesthetic to accurately convey their unauthorized operational state, whilst remaining interactive.

## [G0.1.175] - 2026-04-26

### Fixed

- **Unified Revocation Logic**: The "Revoke" button in the web dashboard is no longer a UI mock. It now triggers a dedicated `/revoke` endpoint that leverages the exact same algorithmic backend as the `xfpm plugin revoke` CLI command, ensuring that the plugin is securely removed from the configuration and correctly re-queued in `pending.plugins.json`.
- **Accurate Revoke Button Visibility**: The web dashboard's "Revoke" button now strictly appears _only_ if a plugin is genuinely authorized.

## [G0.1.174] - 2026-04-26

### Fixed

- **CLI Status Accuracy**: Fixed a bug where a plugin lacking a local installation or signature was incorrectly marked as `VERIFIED` if a config entry existed. It now properly reports as `NOT_INSTALLED` or `KEY_MISMATCH`.
- **UI Toggle State**: Fixed Web UI to accurately leave the `Trust` toggle unchecked for plugins that are still in the `PENDING` state, even in Review Mode.
