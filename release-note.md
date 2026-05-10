## [G0.1.212] - 2026-05-10

### Security & Stability

- **Fixed Critical Resolver Race**: Resolved a `fatal error: concurrent map iteration and map write` in the package resolver. Implemented `sync.RWMutex` on `RegistryPackage` to ensure thread-safe access to cached package metadata during high-concurrency resolution bursts.

## [G0.1.211] - 2026-05-10

### Deep Performance & Concurrency Refactor

- **Atomic Concurrency Scaling**: Replaced lock-based concurrency updates with `CompareAndSwap` (CAS) loops in `DynamicConfig`. This eliminates race windows and ensures precise scaling during high-parallelism bursts.
- **NTFS Pipeline Optimization**:
    - **Pooled Copy Buffers**: Implemented `sync.Pool` for 1MB buffers (Windows) and 256KB (Linux) to reduce GC pressure and maximize sequential throughput.
    - **Reduced Syscall Overhead**: Added `Lstat` guards before `Remove` and batch directory creation to minimize expensive kernel calls on Windows.
    - **Increased Parallelism**: Fine-tuned concurrency ceilings (up to 96 on Windows) to better hide NTFS and Antivirus latency.
- **EWMA Reactivity**: Tuned the Exponentially Weighted Moving Average (EWMA) for latency tracking to react 3x faster to environment improvements.

## [G0.1.210] - 2026-05-10

### Windows Performance Optimizations

- **Directory Creation Cache**: Implemented a global `sync.Map` cache for directory creation across all packages. This eliminates thousands of redundant `MkdirAll` and `Stat` syscalls, which are particularly expensive on Windows NTFS.
- **Optimized Disk I/O**: Removed redundant `Sync()` calls during CAS streaming extraction. By relying on atomic renames, we significantly improve write throughput on Windows without compromising integrity.
- **Terminal Rendering Polish**: Increased the terminal refresh rate on Windows to reduce I/O pressure on the console host, leading to smoother and faster execution.

## [G0.1.209] - 2026-05-10

### Fixes
- **Corrected Update URL**: Fixed an issue where the version check was pointing to a deprecated repository URL, which could lead to incorrect "ahead of release" messages.

## [G0.1.208] - 2026-05-10

### Interactive CLI & Update Flow

- **Fixed Terminal "Cutting" Bug**: Resolved a critical issue where the terminal would return to the shell prompt prematurely during self-updates or after interactive inputs. XFPM now correctly waits for the installer to finish on Windows, providing a stable and sequential upgrade experience.
- **Robust Input Handling**: Hardened the interactive prompt logic to ensure it reliably reads user input across different shell environments (CMD, PowerShell, Bash).

## [G0.1.207] - 2026-05-10

### Windows UX & Stability

- **Binary Renaming Strategy**: Implemented a "hot-swap" update mechanism for Windows. By renaming the running `xfpm.exe` to `xfpm.exe.old`, we can now release the file lock and let the installer replace the binary while keeping the parent process alive. This eliminates the "overlapping prompt" bug where the shell would return to the command line before the installer finished.
- **Auto-Cleanup**: Added automatic cleanup of `.old` binary residues on the next startup.

## [G0.1.206] - 2026-05-10

### Linux/Unix & UX Polish

- **Unix Upgrade Flow**: Restored the sequential upgrade behavior on Linux/macOS. Unlike Windows, Unix systems can handle binary replacement while running, so we now wait for the installer to finish and then re-execute the task automatically.
- **Shadowing Detection**: Improved the Linux installer to detect if `xfpm` is being shadowed by a global installation (e.g., in `/usr/local/bin`) and provide a clear fix command.
- **Log Polish**: Fixed a cosmetic bug where version numbers were prefixed with a double 'v' (`vvG0.1.205`).

## [G0.1.205] - 2026-05-10

### Windows Performance Optimizations

- **Directory Creation Cache**: Implemented a `sync.Map` cache for directory creation during the linking phase. This eliminates thousands of redundant `MkdirAll` syscalls, which are a major performance bottleneck on Windows filesystems.
- **Tuned Concurrency**: Optimized extraction and linking concurrency for Windows (reduced from 16 to 8 simultaneous extractions) to reduce NTFS lock contention and alleviate terminal rendering pressure.
- **Linking Pool Refinement**: Adjusted the global linking pool limit on Windows to ensure smoother I/O operations and more responsive UI updates.

## [G0.1.204] - 2026-05-10

### Windows Self-Update Reliability

- **Non-Blocking Upgrade**: Refactored `xfpm upgrade` on Windows to launch the installer and exit immediately. This releases the binary file lock and prevents the "deadlock" where the installer waits for the process it's trying to replace.
- **Smart Installer Wait**: Updated the PowerShell installer to wait a few seconds for the parent process to exit before prompting to kill it, providing a much smoother upgrade experience.

## [G0.1.203] - 2026-05-10

### Windows Environment Hardening & UX

- **Fixed Binary Linking**: Resolved a critical issue where executables (like `bun.exe`) were incorrectly linked as directory junctions on Windows. XFPM now uses native Hard Links for files, ensuring they are correctly recognized by the OS and shells.
- **Robust Interactive Prompts**: Overhauled `AskYesNo` with `bufio.Scanner` to prevent input-skipping bugs on Windows terminals.
- **Optimized Initialization**: Eliminated redundant "Would you like to install Bun?" prompts during `xfpm init` by implementing internal spawn guards and improved runtime discovery.
- **Path-Aware Bootstrap**: `xfpm init` now correctly detects XFPM-managed runtimes even if the shell hasn't been restarted to refresh the `PATH`.

## [G0.1.202] - 2026-05-10

### Native PowerShell Updater & Installer Hardening

- **Optimized Windows Update**: Migrated the self-update process on Windows from the JS-based bridge to the native `install.ps1` script. This eliminates the "hanging" issues and removes the Node.js requirement for the update sequence.
- **PowerShell StrictMode Fix**: Resolved a `PropertyNotFoundStrict` error in `install.ps1` when detecting running processes, improving reliability on systems with strict execution policies.

## [G0.1.201] - 2026-05-09

### Cross-Platform Interactive CLI

- **Windows Prompt Compatibility**: Implemented a native cross-platform interactive helper (`internal/utils/interactive.go`). This fixes a bug on Windows where interactive prompts (e.g., asking to install Bun) were automatically bypassed and defaulted to "no". The CLI is now fully interactive across both Linux and Windows environments.

## [G0.1.200] - 2026-05-05

### Native XRU Orchestration & Initialization Overhaul

- **Native XRU Integration**: Switched from external CLI execution to native Go library integration using `github.com/Nehonix-Team/xru` (v0.2.5). This eliminates binary dependencies and improves orchestration performance.
- **Enhanced `init` Command**: 
    - **Interactive Setup**: Added a guided configuration wizard for project name, version, description, author, and port when running `xfpm init` without arguments.
    - **Dynamic Arguments**: Support for positional `KEY=VALUE` arguments and a new `--arg` flag to pass custom variables directly to orchestration rules.
    - **Base Metadata Flags**: Added explicit flags for `--author/-a`, `--desc/-d`, `--version/-v`, and `--port/-p`.
- **Resource Management**: Fixed a temporary directory leak during project extraction by implementing strict cleanup of extraction folders.

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
