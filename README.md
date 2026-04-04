# XFPM — XyPriss Fast Package Manager

> High-performance CLI package manager for the XyPriss ecosystem, built in Go.

---

## Table of Contents

- [Overview](#overview)
- [Features](#features)
- [Installation](#installation)
- [Usage](#usage)
- [Architecture](#architecture)
- [License](#license)

---

## Overview

**XFPM** is a cross-platform command-line tool designed for the XyPriss ecosystem. It delivers fast dependency resolution, strict package isolation through a virtual store, and a clean terminal interface suited for professional workflows.

---

## Features

| Feature              | Description                                                                                                     |
| -------------------- | --------------------------------------------------------------------------------------------------------------- |
| **Performance**      | Optimized resolution engine based on a neural dependency graph, written entirely in Go.                         |
| **Strict Isolation** | Content-addressable storage (CAS) and a virtual store architecture prevent dependency leakage between projects. |
| **Cross-Platform**   | Native binaries for Windows, Linux, and macOS — both `amd64` and `arm64`.                                       |
| **Clean Output**     | Structured, minimal terminal feedback with no visual noise.                                                     |
| **Auto-Update**      | Built-in update engine keeps the CLI current without manual intervention.                                       |
| **Single Binary**    | No runtime dependencies. One binary, fully self-contained.                                                      |

---

## Installation

XFPM is distributed through the official **Nehonix unified installer**.

### Unix / macOS / WSL

```bash
curl -sL https://xypriss.nehonix.com/install.js | node
```

### Windows (PowerShell)

```powershell
Invoke-RestMethod -Uri "https://xypriss.nehonix.com/install.js" -UseBasicParsing | node
```

### NPM

> **Note:** Administrative privileges are required to deploy the native binaries.

**Linux / macOS** — `sudo` and the `-g` flag are mandatory. Without `sudo`, the script cannot write to `/usr/local/bin`.

```bash
sudo npm install -g xypriss-cli
```

**Windows** — Run PowerShell or Command Prompt as **Administrator**.

```powershell
npm install -g xypriss-cli
```

---

## Usage

### Initialize a project

```bash
xfpm init
```

### Manage dependencies

```bash
# Install from package.json
xfpm install

# Add a package
xfpm add <package>

# Add to devDependencies
xfpm add -D <package>

# Remove a package
xfpm rm <package>
```

### Update packages

```bash
# Update all packages
xfpm update

# Update a specific package
xfpm update <package>
```

### Package Maintainers: Redirections & Deprecation

Library developers can natively manage package deprecations and redirect seamless installations across the XyPriss ecosystem without breaking workflows. If a package is renamed or deprecated, add an `xfpm` object to the distributed `package.json`:

```json
{
  "name": "old-lib",
  "version": "1.0.0",
  "xfpm": {
    "redirect": {
      "target": "new-lib",
      "message": "old-lib is deprecated. Please refer to our new architecture."
    }
  }
}
```

When users run `xfpm install old-lib`, XFPM intercepts the metadata, displays the custom message in the terminal, and securely shifts the resolution downstream to `new-lib`. The consuming application's `package.json` file is successfully updated to reference the designated `new-lib` target rather than the deprecated package.

---

## Architecture

XFPM resolves dependencies by building a **Neural Dependency Graph** of your project.

### CAS — Content Addressable Storage

Every file is hashed and stored once across the entire system, eliminating duplicates and ensuring deterministic installs.

### Virtual Store

Dependencies are stored by exact version under `node_modules/.xpm/virtual_store` and symlinked into the project's `node_modules`. This enforces strict isolation and prevents dependency leakage between projects.

### Targeted Resolution

Only the modified portions of the dependency graph are recalculated during updates, minimizing overhead and keeping incremental operations fast.

---

## License

Copyright 2025 **Nehonix**. All rights reserved.  
Licensed under the **Nehonix OSL (NOSL)**.

Powered by Nehonix and the XyPriss Engine.
