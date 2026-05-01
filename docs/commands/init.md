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
| `--guardrails` | | `false` | Enable native XHSC network quality protections and latency limits. |
| `--storage` | | `none` | Enable specialized storage engines. Use `xems` for Encrypted Memory Store. |
| `--force` | `-f` | `false` | Force overwrite the target directory if it already exists. |

## How it Works

### 1. Template Discovery
XFPM connects to the GitHub API to fetch the **latest** version of the official `xypriss-templates`. This ensures you always start with the most up-to-date and secure base.

### 2. Orchestration Engine (.xru)
Instead of simply copying files, XFPM processes **XyPriss Rules (`.xru`)**. These rules allow for:
- **JSON Patching**: Deep-merging configurations into `package.json` or `xypriss.config.jsonc`.
- **TypeScript Injection**: Injecting specialized code blocks (like XEMS routes) into your source files via `@TSINJECT` markers.
- **Dynamic Creation**: Generating project-specific files on the fly.

### 3. Automated Setup
After the orchestration is complete, XFPM:
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
