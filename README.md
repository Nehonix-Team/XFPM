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
| **Security Audit**   | Standalone SCA engine with premium interactive reports and decentralized revocation enforcement.                |
| **Zero-Trust G3**    | Ed25519 cryptographic signing and verification layer for the secure plugin ecosystem.                           |
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

### Dependency Audit

Audit your dependencies for known vulnerabilities via the OSV database and automatically repair them.

```bash
# Standard interactive audit
xfpm audit

# Advanced repair loop
xfpm audit fix

# Force a specific report mode
xfpm audit --tree  # Terminal tree view
xfpm audit --html  # Open interactive XFPML dashboard
```

#### Intelligent Fix Loop

The `audit fix` command performs a multi-step remediation:

1. **Registry Validation**: Compares local versions with NPM registry `latest`.
2. **Automated Repair**: Updates `package.json` and performs a clean installation.
3. **Re-Verification**: Automatically re-audits the project to confirm the fix is effective.
4. **Fallback Uninstallation**: Offers to remove packages that remain vulnerable even at their latest versions.

Use `--yes` and `--force-remove` for fully automated security enforcement in CI environments.

### Inspect Dependencies

````bash
# List all direct dependencies
xfpm list

# List specific packages and their dependents
xfpm list <package...>

### Security & Signing (Zero-Trust G3)

XFPM enforces a cryptographically verified security model for the XyPriss ecosystem.

#### 1. Generate Identity
Authors must generate a unique Ed25519 developer identity before signing plugins.
```bash
xfpm gen-key
````

Your public key fingerprint should be published in your plugin's official README to allow users to verify your identity.

#### 2. Sign Plugin Assets

Before publication, generate a tamper-proof signature manifest.

```bash
xfpm sign ./ --min-version 1.0.0
```

This hashes all production files and creates a `xypriss.plugin.sig` file required for secure distribution.

#### 3. Trust Verification (TOFU)

During installation of a new plugin, XFPM activates an interactive **Trust On First Use** flow, prompting the administrator to authorize the Developer ID and pin it to the project configuration.

### Dependency Audit & Revocation

Audit your dependencies for known vulnerabilities and enforced framework revocations.

```bash
# Standard interactive audit
xfpm audit

# Advanced repair loop
xfpm audit fix
```

#### Decentralized Revocation Enforcement

XFPM tracks framework-level revocations via native package metadata. If a version is discovered to be compromised:

- **Audit Flagging**: `xfpm audit` will mark the package as revoked.
- **Runtime Patching**: XFPM injects an `xfpm.revoked` marker into the local `package.json`, which is natively caught by the XHSC Deep Audit engine to block execution.

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

Every file is hashed and stored once across the entire system within the global `~/.xpm/storage` directory, eliminating duplicates and ensuring deterministic installs.

### Virtual Store

Dependencies are rigorously stored by exact version under the project-local `node_modules/.xpm/vstore` and symlinked into the project's root `node_modules`. This "Ancestor Hoisting" architecture enforces strict isolation while avoiding ghost dependencies.

### Targeted Resolution

Only the modified portions of the dependency graph are recalculated during updates, minimizing overhead and keeping incremental operations fast.

---

## License

Copyright 2025 **Nehonix**. All rights reserved.  
Licensed under the **Nehonix OSL (NOSL)**.

Powered by Nehonix.
