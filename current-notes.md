## [G0.1.202] - 2026-05-10

### Native PowerShell Updater & Installer Hardening

- **Optimized Windows Update**: Migrated the self-update process on Windows from the JS-based bridge to the native `install.ps1` script. This eliminates the "hanging" issues and removes the Node.js requirement for the update sequence.
- **PowerShell StrictMode Fix**: Resolved a `PropertyNotFoundStrict` error in `install.ps1` when detecting running processes, improving reliability on systems with strict execution policies.

