## [G0.1.219] - 2026-06-04

### CLI Enhancements

- **Local Web Dashboard Flag**: Added a new `-w` / `--web` flag to the `xfpm plugin list` command. This enables opening the verification dashboard in the browser without enforcing the network checks and orphan discovery inherent to the `-r` (`--review`) mode. It fully supports offline use when combined with the local flag (`-lw`).

