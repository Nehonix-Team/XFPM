# xfpm init

The `xfpm init` command is the entry point for creating new XyPriss projects. It uses a sophisticated **Orchestration Engine** to dynamically assemble project structures based on declarative rules (`.xru`).

## Usage

```bash
xfpm init [flags]
```

By default, running `xfpm init` will create a new directory named `my-xypriss-app` in your current working directory using the `default` mode and `standard` security profile.

## Flags

| Flag | Shorthand | Default | Description |
| :--- | :--- | :--- | :--- |
| `--name` | `-n` | `my-xypriss-app` | The name of the project and the target directory. |
| `--mode` | `-m` | `default` | Orchestration mode. Use `default` for monolithic apps or `xms` for Multi-Server architectures. |
| `--security` | `-s` | `standard` | Security profile. Options: `standard`, `api` (CLI/Terminal focus), or `web` (Strict CSP). |
| `--guardrails` | `-g` | `false` | Enable native XHSC network quality protections and latency limits. |
| `--storage` | `-S` | `none` | Enable specialized storage engines. Use `xems` for Encrypted Memory Store. |
| `--desc` | `-d` | | Short description of the project (replaces `{{DESCRIPTION}}` in templates). |
| `--author` | `-a` | `XyPriss Developer`| Author name (replaces `{{AUTHOR}}` in templates). |
| `--version` | `-v` | `1.0.0` | Initial project version (replaces `{{VERSION}}` in templates). |
| `--alias` | `-A` | | Project alias (replaces `{{ALIAS}}` in templates). |
| `--port` | `-p` | `8080` | Default server port (replaces `{{PORT}}` in templates). |
| `--main-port` | | `8081` | XMS Main server port (replaces `{{MAIN_PORT}}` in templates). |
| `--auth-port` | | `8082` | XMS Auth server port (replaces `{{AUTH_PORT}}` in templates). |
| `--force` | `-f` | `false` | Force overwrite the target directory if it already exists. |

## How it Works

### 1. Template Discovery
XFPM connects to the GitHub API to fetch the **latest** version of the official `xypriss-templates`. This ensures you always start with the most up-to-date and secure base.

### 2. Orchestration Engine (.xru)
Instead of simply copying files, XFPM processes **XyPriss Rules (`.xru`)**. These rules allow for:
- **JSON Patching**: Deep-merging configurations into `package.json` or `xypriss.config.jsonc`.
- **TypeScript Injection**: Injecting specialized code blocks (like XEMS routes) into your source files via `@TSINJECT` markers.
- **Dynamic Creation**: Generating project-specific files on the fly.

### 3. Global Variable Injection
XFPM scans all project files and replaces specific placeholders with your provided metadata:
- `{{SERVER_NAME}}` → Project Name
- `{{DESCRIPTION}}` → Project Description
- `{{AUTHOR}}` → Author Name
- `{{VERSION}}` → Project Version
- `{{ALIAS}}` → Project Alias
- `{{PORT}}` → Default Port
- `{{MAIN_PORT}}` → XMS Main Port
- `{{AUTH_PORT}}` → XMS Auth Port

### 4. Automated Setup
After the orchestration and injection are complete, XFPM:
1. Verifies the presence of **Bun** (and installs it via XFPM if missing).
2. Runs a full `xfpm install` to pull in all required dependencies.
3. Initializes the neural dependency graph.

## Examples

### Create a standard web application
```bash
xfpm init --name my-web-app --security web
```

### Create a high-performance Multi-Server (XMS) project with XEMS
```bash
xfpm init --name cloud-core --mode xms --storage xems --guardrails
```

---
*Powered by XyPriss Orchestration Engine*
