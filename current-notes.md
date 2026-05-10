## [G0.1.203] - 2026-05-10

### Windows Environment Hardening & UX

- **Fixed Binary Linking**: Resolved a critical issue where executables (like `bun.exe`) were incorrectly linked as directory junctions on Windows. XFPM now uses native Hard Links for files, ensuring they are correctly recognized by the OS and shells.
- **Robust Interactive Prompts**: Overhauled `AskYesNo` with `bufio.Scanner` to prevent input-skipping bugs on Windows terminals.
- **Optimized Initialization**: Eliminated redundant "Would you like to install Bun?" prompts during `xfpm init` by implementing internal spawn guards and improved runtime discovery.
- **Path-Aware Bootstrap**: `xfpm init` now correctly detects XFPM-managed runtimes even if the shell hasn't been restarted to refresh the `PATH`.

