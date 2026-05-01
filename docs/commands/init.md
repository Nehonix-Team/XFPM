# xfpm init

The `init` command is used to bootstrap a new **XyPriss** project. It automates the creation of the project structure, configures essential metadata, and prepares the development environment with high-performance defaults.

## Usage

```bash
xfpm init --name <name> --desc <description> --version <version> [flags]
```

To create a new project, you must provide the project name, a brief description, and the initial version number.

## Options

| Option | Shorthand | Default | Description |
| :--- | :--- | :--- | :--- |
| `--name` | `-n` | (Required) | The name of the project and its directory. |
| `--desc` | `-d` | (Required) | A short description of the project's purpose. |
| `--version` | `-v` | (Required) | The initial version string (e.g., `{{VERSION}}`). |
| `--author` | `-a` | `XyPriss Developer` | The name of the project author. |
| `--alias` | `-A` | | An internal identifier or alias for the application. |
| `--mode` | `-m` | `default` | Project architecture: `default` (standard) or `xms` (Multi-Server). |
| `--security` | `-s` | `standard` | Security profile: `standard`, `api`, or `web`. |
| `--port` | `-p` | `8080` | The primary network port for the application. |
| `--guardrails` | `-g` | `false` | Enable network guardrails for latency and traffic control. |
| `--storage` | `-S` | `none` | Enable specialized storage engines like `xems`. |
| `--force` | `-f` | `false` | Overwrite the target directory if it already exists. |

## Feature Highlights

### Dynamic Project Modes
- **Default**: A streamlined, high-performance architecture for standard applications.
- **XMS (Multi-Server)**: A scalable architecture for distributed systems. When using XMS, XFPM automatically manages port assignments starting from your provided `--port`.

### Automated Environment Setup
Upon initialization, XFPM automatically:
1. Provisions the project structure from the latest official templates.
2. Configures your metadata across the entire codebase.
3. Verifies and installs the **Bun** runtime if necessary.
4. Performs an initial `xfpm install` to ensure all dependencies are ready.

