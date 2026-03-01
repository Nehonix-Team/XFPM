# XFPM — XyPriss Fast Package Manager

**XFPM** is a high-performance, cross-platform CLI tool built for the XyPriss ecosystem. Written in Go, it delivers fast dependency resolution, strict package isolation through a virtual store, and a clean terminal interface designed for professional workflows.

---

## Features

- **Performance** — Optimized resolution engine based on a neural dependency graph, written entirely in Go.
- **Strict Isolation** — Content-addressable storage (CAS) and a virtual store architecture prevent dependency leakage between projects.
- **Cross-Platform** — Native binaries available for Windows, Linux, and macOS (amd64 and arm64).
- **Clean Output** — Structured, minimal terminal feedback with no visual noise.
- **Auto-Update** — Built-in update engine keeps the CLI current without manual intervention.
- **Single Binary** — No runtime dependencies. One binary, fully self-contained.

---

## Installation

XFPM is distributed through the official Nehonix unified installer.

### Unix / macOS / WSL

```bash
curl -sL https://xypriss.nehonix.com/install.js | node
```

### Windows (PowerShell)

```powershell
Invoke-RestMethod -Uri "https://xypriss.nehonix.com/install.js" -UseBasicParsing | node
```

---

## Usage

### Initialize a project

```bash
xfpm init
```

### Install dependencies

```bash
xfpm install           # Install from package.json
xfpm add axios         # Add a specific package
xfpm add -D vitest     # Add to devDependencies
```

### Update packages

```bash
xfpm update            # Update all packages
xfpm update axios      # Update a specific package
```

### Remove a package

```bash
xfpm rm <package>
```

---

## Architecture

XFPM resolves dependencies by building a **Neural Dependency Graph** of your project.

1. **CAS (Content Addressable Storage)** — Every file is hashed and stored once, eliminating duplicates across the system.
2. **Virtual Store** — Dependencies are stored in a hidden directory (`node_modules/.xpm/virtual_store`) by exact version and symlinked into the project's `node_modules`.
3. **Targeted Resolution** — Only the modified portions of the dependency graph are recalculated during updates, minimizing overhead.

---

## License

Copyright 2025 Nehonix. All rights reserved.  
Licensed under the **Nehonix OSL (NOSL)**.

Powered by Nehonix and the XyPriss Engine.
