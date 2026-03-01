# xypriss-cli (NPM Compatibility Wrapper)

This package is a compatibility wrapper designed to install the native **XyPriss Fast Package Manager (XFPM)** on your system. It triggers the official native deployment sequence to ensure maximum performance and correct system integration.

## Installation

**CRITICAL: Administrative privileges are required to install the native binaries.**

### Linux / macOS

You **must** use `sudo` and the global flag `-g`. Failure to use `sudo` will result in an installation error as the script needs to deploy binaries to `/usr/local/bin`.

```bash
sudo npm install -g xypriss-cli
```

### Windows

Run your terminal (PowerShell or Command Prompt) as **Administrator** and use the global flag `-g`.

```powershell
npm install -g xypriss-cli
```

---

## Why is this necessary?

XFPM is a high-performance binary written in Go. Unlike standard NPM packages, it needs to:

1.  Install a standalone native binary to your system.
2.  Configure system-wide aliases (`xfpm`, `xyp`, `xypcli`).
3.  Ensure the binaries are available in your system PATH.

By using the global installation with administrative rights, you ensure that XyPriss is correctly optimized and accessible from any directory.

---

**Powered by Nehonix**
village
