## [G0.1.215] - 2026-05-14

### Official Stable Release & UI
- **Official Graduation**: XyPriss has officially moved out of Beta.
- **Enhanced `info` Command**: Complete UI overhaul for the `info` command, providing rich metadata including description, author, license, and keywords in a premium boxed layout.
- **Path Validation**: Hardened the `add` command with strict local path validation to prevent silent failures on invalid plugin paths.
- **Symlink Race Fix**: Implemented a thread-safe blocking mechanism for directory creation, resolving race conditions during high-concurrency symlink operations (especially for scoped packages).

