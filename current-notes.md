## [G0.1.210] - 2026-05-10

### Windows Performance Optimizations

- **Directory Creation Cache**: Implemented a global `sync.Map` cache for directory creation across all packages. This eliminates thousands of redundant `MkdirAll` and `Stat` syscalls, which are particularly expensive on Windows NTFS.
- **Optimized Disk I/O**: Removed redundant `Sync()` calls during CAS streaming extraction. By relying on atomic renames, we significantly improve write throughput on Windows without compromising integrity.
- **Terminal Rendering Polish**: Increased the terminal refresh rate on Windows to reduce I/O pressure on the console host, leading to smoother and faster execution.

