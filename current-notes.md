## [G0.1.211] - 2026-05-10

### Deep Performance & Concurrency Refactor

- **Atomic Concurrency Scaling**: Replaced lock-based concurrency updates with `CompareAndSwap` (CAS) loops in `DynamicConfig`. This eliminates race windows and ensures precise scaling during high-parallelism bursts.
- **NTFS Pipeline Optimization**:
    - **Pooled Copy Buffers**: Implemented `sync.Pool` for 1MB buffers (Windows) and 256KB (Linux) to reduce GC pressure and maximize sequential throughput.
    - **Reduced Syscall Overhead**: Added `Lstat` guards before `Remove` and batch directory creation to minimize expensive kernel calls on Windows.
    - **Increased Parallelism**: Fine-tuned concurrency ceilings (up to 96 on Windows) to better hide NTFS and Antivirus latency.
- **EWMA Reactivity**: Tuned the Exponentially Weighted Moving Average (EWMA) for latency tracking to react 3x faster to environment improvements.

