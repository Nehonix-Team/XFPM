# xfpm init

The `init` command is used to bootstrap a new **XyPriss** project. It leverages the native **XRU (XyPriss Rule Unit)** orchestration engine to automate structure creation, metadata injection, and environment preparation.

## Usage

```bash
xfpm init [name] [KEY=VALUE...] [flags]
```

### Examples
- **Interactive**: `xfpm init` (Follow the guided wizard)
- **Quick Start**: `xfpm init my-app`
- **Full Configuration**: `xfpm init my-app AUTHOR="Nehonix" VERSION=2.0.0 PORT=9000`
- **With Flags**: `xfpm init my-app --mode xms --force`

## Options

| Option | Shorthand | Default | Description |
| :--- | :--- | :--- | :--- |
| `--mode` | `-m` | `default` | Orchestration mode: `default` or `xms` (Multi-Server). |
| `--author` | `-a` | `Nehonix-Team` | The name of the project author. |
| `--desc` | `-d` | (Generic) | A short description of the project. |
| `--version` | `-v` | `1.0.0` | Initial project version. |
| `--port` | `-p` | `8080` | Default network port for the server. |
| `--arg` | | | Pass custom orchestration variables in `KEY=VALUE` format. |
| `--force` | `-f` | `false` | Overwrite the target directory if it already exists. |

## Feature Highlights

### Guided Interactive Mode
If run without arguments, `xfpm init` launches an interactive wizard that guides you through the essential configuration (Name, Version, Description, Author, Port). It provides sane defaults to get you started in seconds.

### Dynamic Orchestration Variables
XFPM supports a powerful "Pass-Through" argument system. Any positional argument containing an `=` (e.g., `THEME=dark`) is automatically captured and injected into the XRU orchestration rules. This allows you to customize templates without ever modifying the XFPM binary.

### Native XRU Integration
As of **vG0.1.200**, the orchestration engine is natively integrated.
- **No external dependencies**: Works out of the box without installing the XRU CLI.
- **Atomic Execution**: Ensures all rules are applied consistently before finalizing the project.
- **Cross-Platform**: Handles pathing and shell operations natively on Windows, Linux, and macOS.

### Automated Environment Setup
1. **Template Provisioning**: Fetches the latest high-performance templates from official repositories.
2. **Metadata Injection**: Recursively updates all configuration files, READMEs, and manifests.
3. **Runtime Validation**: Verifies and installs the **Bun** runtime if missing.
4. **Auto-Install**: Performs a full dependency installation so you're ready for `xfpm dev` immediately.

