# XFPM - Release Notes

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
