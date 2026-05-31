## [G0.1.217] - 2026-05-31

### Silent Mode

- **New `--silent / -S` Flag**: Added a global persistent flag available on all commands (including `install`, `add`, `update`). When active, all informational output is suppressed: spinners, progress bars, log prefixes, lifecycle script streams, and the legacy-store migration prompt.
- **CI/CD Ready**: The flag is designed for scripted and pipeline environments where terminal noise is undesirable. Fatal errors (non-zero exit codes) are never suppressed.
- **`utils.SetSilent(bool)`**: Introduced a single source of truth in the logger. Calling `SetSilent(true)` disables pterm output globally via `pterm.DisableOutput()` and turns every log function (`Info`, `Success`, `Warn`, `Log`, `LiveLog`, `Matrix`, `Premium`, `PrintFooter`) into a no-op.
- **Progress Bar Redirection**: The `mpb` progress bar instance in the installer is redirected to `io.Discard` when silent, eliminating all terminal rendering without altering the concurrency or download logic.
- **Script Runner**: In silent mode, lifecycle script stdout/stderr pipes are drained to `io.Discard` to prevent child process write-blocking, while producing no terminal output.

