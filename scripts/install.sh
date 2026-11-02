#!/usr/bin/env bash
# XFPM (XyPriss Fast Package Manager) — Unix/macOS/Linux Installer
# Supports: Linux (x64, arm64), macOS (x64, arm64)
#
# Usage:
#   curl -fsSL https://dll.nehonix.com/repo/xypriss/xfpm/scripts/install.sh | bash
#   curl -fsSL https://dll.nehonix.com/repo/xypriss/xfpm/scripts/install.sh | bash -s -- --version 1.2.0
#   curl -fsSL https://dll.nehonix.com/repo/xypriss/xfpm/scripts/install.sh | bash -s -- uninstall

set -euo pipefail

# ─────────────────────────────────────────────────────────────
# Constants
# ─────────────────────────────────────────────────────────────
REPO="Nehonix-Team/XFPM"
BRIDGE_URL="https://dll.nehonix.com/repo/xypriss/xfpm/scripts/bridge.js"
TOOL_NAME="xfpm"
ALIASES=("xyp" "xypcli" "dev" "xfpmc" "xfpmx")
VERSION="${XFPM_VERSION:-latest}"
if [ "${EUID:-$(id -u)}" -eq 0 ] && [ -z "${XFPM_INSTALL:-}" ]; then
  XFPM_ROOT="/usr/local/share/xfpm"
else
  XFPM_ROOT="${XFPM_INSTALL:-$HOME/.xfpm}"
fi
XFPM_BIN="$XFPM_ROOT/bin"
tmp_dir=""

# ─────────────────────────────────────────────────────────────
# Parse args  (install | uninstall | --version <v> | --no-path)
# ─────────────────────────────────────────────────────────────
ACTION="install"
NO_PATH=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    uninstall) ACTION="uninstall"; shift ;;
    xru|--xru) ACTION="xru"; shift ;;
    --version) VERSION="$2"; shift 2 ;;
    --no-path) NO_PATH=true; shift ;;
    *) shift ;;
  esac
done

# ─────────────────────────────────────────────────────────────
# Colors — only if stdout is a terminal
# ─────────────────────────────────────────────────────────────
if [ -t 1 ]; then
  C_RESET="\033[0m"
  C_RED="\033[0;31m"
  C_GREEN="\033[1;32m"
  C_CYAN="\033[0;36m"
  C_YELLOW="\033[1;33m"
  C_BOLD="\033[1m"
  C_DIM="\033[2m"
else
  C_RESET="" C_RED="" C_GREEN="" C_CYAN="" C_YELLOW="" C_BOLD="" C_DIM=""
fi

# ─────────────────────────────────────────────────────────────
# Display helpers
# ─────────────────────────────────────────────────────────────
info()    { echo -e "${C_CYAN}info${C_RESET}  $*"; }
ok()      { echo -e "${C_GREEN}ok${C_RESET}    $*"; }
warn()    { echo -e "${C_YELLOW}warn${C_RESET}  $*" >&2; }
fatal()   {
  echo -e "\n${C_RED}${C_BOLD}Install Failed${C_RESET}" >&2
  echo -e "  $*\n" >&2
  exit 1
}

# ─────────────────────────────────────────────────────────────
# OS / arch detection
# ─────────────────────────────────────────────────────────────
detect_platform() {
  local os arch

  os="$(uname -s)"
  arch="$(uname -m)"

  case "$os" in
    Linux)  OS_NAME="linux"  ;;
    Darwin) OS_NAME="darwin" ;;
    *)      fatal "Unsupported OS: $os. XFPM supports Linux and macOS." ;;
  esac

  case "$arch" in
    x86_64)         ARCH_NAME="amd64" ;;
    aarch64|arm64)  ARCH_NAME="arm64" ;;
    *)              fatal "Unsupported architecture: $arch. XFPM supports x86_64 and arm64." ;;
  esac

  BINARY_NAME="xfpm-${OS_NAME}-${ARCH_NAME}"
}

# ─────────────────────────────────────────────────────────────
# Dependency checks
# ─────────────────────────────────────────────────────────────
check_downloader() {
  if command -v curl >/dev/null 2>&1; then
    DOWNLOADER="curl"
  elif command -v wget >/dev/null 2>&1; then
    DOWNLOADER="wget"
  else
    fatal "Neither curl nor wget is available. Please install one and try again."
  fi
}

# Returns: "node", "deno", "bun", or ""
detect_js_runtime() {
  if command -v node >/dev/null 2>&1; then
    # Make sure it's recent enough (v14+)
    local node_major
    node_major=$(node -e "process.stdout.write(process.versions.node.split('.')[0])" 2>/dev/null || echo "0")
    if [ "$node_major" -ge 14 ]; then
      echo "node"; return
    else
      warn "Node.js $(node --version) is too old (need v14+). Trying other runtimes..."
    fi
  fi
  if command -v deno >/dev/null 2>&1; then echo "deno"; return; fi
  if command -v bun  >/dev/null 2>&1; then echo "bun";  return; fi
  echo ""
}

# ─────────────────────────────────────────────────────────────
# Download helper (curl or wget)
# ─────────────────────────────────────────────────────────────
download() {
  local url="$1" dest="$2"
  if [ "$DOWNLOADER" = "curl" ]; then
    curl --retry 3 --retry-delay 2 --retry-connrefused \
         -fsSL --progress-bar "$url" -o "$dest"
  else
    wget -q --show-progress --tries=3 --waitretry=2 \
         "$url" -O "$dest"
  fi
}

# ─────────────────────────────────────────────────────────────
# Run bridge.js with the best available JS runtime
# ─────────────────────────────────────────────────────────────
run_bridge() {
  local bridge_path="$1" runtime="$2"
  local out_path="$XFPM_BIN/xfpm"

  local bridge_args="--out $out_path"
  [ "$VERSION" != "latest" ] && bridge_args="$bridge_args --version $VERSION"

  info "Running bridge via $runtime..."

  case "$runtime" in
    node) XFPM_OUT="$out_path" XFPM_VERSION="$VERSION" node "$bridge_path" $bridge_args ;;
    deno) XFPM_OUT="$out_path" XFPM_VERSION="$VERSION" deno run --allow-net --allow-write --allow-read --allow-env "$bridge_path" $bridge_args ;;
    bun)  XFPM_OUT="$out_path" XFPM_VERSION="$VERSION" bun  run "$bridge_path" $bridge_args ;;
  esac
}

# ─────────────────────────────────────────────────────────────
# Fallback: download binary directly without bridge (curl/wget)
# Used when no JS runtime is available at all
# ─────────────────────────────────────────────────────────────
fallback_direct_download() {
  warn "No JS runtime found (node/deno/bun). Falling back to direct download via $DOWNLOADER..."

  local version="$VERSION"
  # Normalize version
  if [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then version="xfpm-v$version"; fi
  if [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then version="xfpm-$version"; fi

  local segment
  [ "$version" = "latest" ] && segment="latest/download" || segment="download/$version"

  local url="https://github.com/${REPO}/releases/${segment}/${BINARY_NAME}"
  local dest="$XFPM_BIN/xfpm"
  local tmp="${dest}.tmp"

  info "Downloading from: $url"
  download "$url" "$tmp"

  if [ ! -f "$tmp" ] || [ ! -s "$tmp" ]; then
    rm -f "$tmp"
    fatal "Downloaded file is missing or empty. Check your internet connection."
  fi

  mv "$tmp" "$dest"
  chmod +x "$dest"
}

# ─────────────────────────────────────────────────────────────
# Add XFPM_BIN to shell profile (bash/zsh/fish)
# ─────────────────────────────────────────────────────────────
add_to_path() {
  if [ "$NO_PATH" = true ]; then
    info "Skipping PATH update (--no-path)."
    return
  fi

  # Already in PATH?
  if echo "$PATH" | tr ':' '\n' | grep -qxF "$XFPM_BIN"; then
    info "$XFPM_BIN is already in PATH."
    return
  fi

  local export_line="export PATH=\"$XFPM_BIN:\$PATH\""
  local added=false

  for profile in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$profile" ]; then
      # Avoid duplicates
      if ! grep -qF "$XFPM_BIN" "$profile" 2>/dev/null; then
        {
          echo ""
          echo "# XFPM — added by installer"
          echo "$export_line"
        } >> "$profile"
        ok "PATH updated in $profile"
        added=true
      fi
    fi
  done

  # Fish shell
  if command -v fish >/dev/null 2>&1; then
    local fish_config="$HOME/.config/fish/config.fish"
    if [ -f "$fish_config" ] && ! grep -qF "$XFPM_BIN" "$fish_config" 2>/dev/null; then
      {
        echo ""
        echo "# XFPM — added by installer"
        echo "fish_add_path $XFPM_BIN"
      } >> "$fish_config"
      ok "PATH updated in $fish_config"
      added=true
    fi
  fi

  if [ "$added" = false ]; then
    warn "Could not find a shell profile to update. Add this manually:"
    warn "  $export_line"
  fi
}

# ─────────────────────────────────────────────────────────────
# Install aliases:
#   xyp, xypcli  → symlinks vers xfpm
#   xfpmc        → wrapper script pour "xfpm create ..."
#   xfpmx        → wrapper script pour "xfpm exec ..."
#   dev          → wrapper script pour "xfpm dev ..."
# ─────────────────────────────────────────────────────────────
install_aliases() {
  # Symlinks simples
  for alias in "xyp" "xypcli"; do
    ln -sf "$XFPM_BIN/$TOOL_NAME" "$XFPM_BIN/$alias"
    ok "Alias created: $alias → xfpm"
  done

  # Wrappers avec sous-commande intégrée
  declare -A WRAPPERS=( ["xfpmc"]="create" ["xfpmx"]="exec" ["dev"]="dev" )
  for alias in "${!WRAPPERS[@]}"; do
    local subcmd="${WRAPPERS[$alias]}"
    local wrapper="$XFPM_BIN/$alias"
    cat > "$wrapper" <<EOF
#!/usr/bin/env sh
exec "$XFPM_BIN/$TOOL_NAME" $subcmd "\$@"
EOF
    chmod +x "$wrapper"
    ok "Alias created: $alias → xfpm $subcmd"
  done
}

# ─────────────────────────────────────────────────────────────
# Install system symlinks into /usr/local/bin
# Enables 'sudo xfpm' and system-wide execution
# ─────────────────────────────────────────────────────────────
install_system_symlinks() {
  local target_dir="/usr/local/bin"
  local bins=("$TOOL_NAME" "${ALIASES[@]}")

  if [ ! -d "$target_dir" ]; then
    return 0
  fi

  info "Configuring system-wide access in $target_dir (for sudo)..."

  local can_write=false
  local use_sudo=false

  if [ -w "$target_dir" ]; then
    can_write=true
  elif command -v sudo >/dev/null 2>&1; then
    if sudo -n true 2>/dev/null; then
      can_write=true
      use_sudo=true
    elif [ -c /dev/tty ] && { [ -t 0 ] || [ -t 1 ]; }; then
      info "Requesting sudo privileges to enable 'sudo xfpm' system-wide..."
      if sudo -v < /dev/tty 2>/dev/null; then
        can_write=true
        use_sudo=true
      fi
    fi
  fi

  if [ "$can_write" = true ]; then
    for b in "${bins[@]}"; do
      local src="$XFPM_BIN/$b"
      local dst="$target_dir/$b"
      if [ -e "$src" ] || [ -L "$src" ]; then
        if [ "$use_sudo" = true ]; then
          sudo rm -f "$dst" 2>/dev/null || true
          if sudo ln -sf "$src" "$dst" 2>/dev/null; then
            ok "System link created: $dst → $src"
          fi
        else
          rm -f "$dst" 2>/dev/null || true
          if ln -sf "$src" "$dst" 2>/dev/null; then
            ok "System link created: $dst → $src"
          fi
        fi
      fi
    done
  else
    warn "Could not create system symlinks in $target_dir (permissions required)."
    warn "To enable 'sudo xfpm', run manually:"
    warn "  sudo ln -sf \"$XFPM_BIN/$TOOL_NAME\" \"$target_dir/$TOOL_NAME\""
    for b in "${ALIASES[@]}"; do
      warn "  sudo ln -sf \"$XFPM_BIN/$b\" \"$target_dir/$b\""
    done
  fi
}

# ─────────────────────────────────────────────────────────────
# Remove system symlinks from /usr/local/bin
# ─────────────────────────────────────────────────────────────
remove_system_symlinks() {
  local target_dir="/usr/local/bin"
  local bins=("$TOOL_NAME" "${ALIASES[@]}")

  if [ ! -d "$target_dir" ]; then
    return 0
  fi

  local use_sudo=false
  if [ ! -w "$target_dir" ] && command -v sudo >/dev/null 2>&1; then
    if sudo -n true 2>/dev/null; then
      use_sudo=true
    elif [ -c /dev/tty ] && { [ -t 0 ] || [ -t 1 ]; }; then
      sudo -v < /dev/tty 2>/dev/null && use_sudo=true || true
    fi
  fi

  for b in "${bins[@]}"; do
    local dst="$target_dir/$b"
    if [ -L "$dst" ]; then
      local target
      target="$(readlink "$dst" 2>/dev/null || true)"
      if [[ "$target" == *"$XFPM_ROOT"* ]] || [[ "$target" == *"$TOOL_NAME"* ]]; then
        if [ "$use_sudo" = true ]; then
          sudo rm -f "$dst" 2>/dev/null || true
        else
          rm -f "$dst" 2>/dev/null || true
        fi
        ok "Removed system symlink: $dst"
      fi
    fi
  done
}

# ─────────────────────────────────────────────────────────────
# Verify the binary runs
# ─────────────────────────────────────────────────────────────
verify_binary() {
  local bin="$XFPM_BIN/$TOOL_NAME"

  if [ ! -f "$bin" ]; then
    fatal "Binary not found at $bin after installation."
  fi
  chmod +x "$bin" 2>/dev/null || true
  if [ ! -x "$bin" ]; then
    fatal "Binary at $bin is not executable."
  fi

  local ver
  ver="$("$bin" --version 2>&1)" || fatal "Binary verification failed. The binary may be corrupted or incompatible with your system."
  echo "$ver"
}

# ─────────────────────────────────────────────────────────────
# Uninstall
# ─────────────────────────────────────────────────────────────
uninstall_xfpm() {
  info "Uninstalling XFPM..."

  local removed=false

  # Remove binaries and aliases from install dir
  for bin in "$TOOL_NAME" "${ALIASES[@]}"; do
    local path="$XFPM_BIN/$bin"
    if [ -f "$path" ] || [ -L "$path" ]; then
      rm -f "$path"
      ok "Removed: $path"
      removed=true
    fi
  done

  # Remove from shell profiles
  for profile in "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zshrc" "$HOME/.profile"; do
    if [ -f "$profile" ] && grep -qF "$XFPM_BIN" "$profile" 2>/dev/null; then
      # Remove the 3-line block added by installer
      sed -i.bak "/# XFPM — added by installer/{N;N;d;}" "$profile" 2>/dev/null || true
      rm -f "${profile}.bak"
      ok "Cleaned PATH entry from $profile"
    fi
  done

  # Fish shell
  if [ -f "$HOME/.config/fish/config.fish" ]; then
    sed -i.bak '/# XFPM — added by installer/{N;N;d;}' "$HOME/.config/fish/config.fish" 2>/dev/null || true
    rm -f "$HOME/.config/fish/config.fish.bak"
  fi

  # Remove system symlinks in /usr/local/bin
  remove_system_symlinks

  # Remove the entire install dir if empty
  if [ -d "$XFPM_ROOT" ]; then
    rm -rf "$XFPM_ROOT"
    ok "Removed install directory: $XFPM_ROOT"
  fi

  if [ "$removed" = false ]; then
    warn "No XFPM installation found at $XFPM_BIN."
  else
    ok "XFPM has been uninstalled successfully."
  fi
}

# ─────────────────────────────────────────────────────────────
# Main install flow
# ─────────────────────────────────────────────────────────────
install_xfpm() {
  echo ""
  echo -e "${C_BOLD}${C_CYAN}XFPM Installer${C_RESET}"
  echo -e "${C_CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"

  # 1. Detect platform
  detect_platform
  info "Platform : ${OS_NAME}/${ARCH_NAME}"
  info "Version  : ${VERSION}"

  # 2. Check downloader
  check_downloader
  info "Downloader: $DOWNLOADER"

  # 3. Handle existing installation
  if [ -f "$XFPM_BIN/$TOOL_NAME" ]; then
    warn "Existing XFPM installation found at $XFPM_BIN/$TOOL_NAME"

    local choice="y"
    if [ -t 0 ]; then
      read -r -p "    Overwrite / update? (Y/n): " choice < /dev/tty || choice="y"
      choice="${choice:-y}"
    else
      warn "Non-interactive environment — proceeding with update..."
    fi

    case "$choice" in
      [nN]) info "Installation cancelled."; exit 0 ;;
      *)
        rm -f "$XFPM_BIN/$TOOL_NAME"
        for alias in "${ALIASES[@]}"; do rm -f "$XFPM_BIN/$alias"; done
        ;;
    esac
  fi

  # 4. Create install directory
  mkdir -p "$XFPM_BIN"
  chmod 755 "$XFPM_ROOT" "$XFPM_BIN" 2>/dev/null || true
  # 5. Detect JS runtime
  local runtime
  runtime="$(detect_js_runtime)"

  if [ -n "$runtime" ]; then
    # 6a. Download bridge.js into a temp dir
    # Note: tmp_dir is not local so that the EXIT trap can access it
    tmp_dir="$(mktemp -d)"
    trap 'rm -rf "$tmp_dir"' EXIT

    local bridge_path="$tmp_dir/bridge.js"
    info "Downloading bridge from: $BRIDGE_URL"
    download "$BRIDGE_URL" "$bridge_path"

    if [ ! -f "$bridge_path" ] || [ ! -s "$bridge_path" ]; then
      fatal "Failed to download bridge.js from $BRIDGE_URL"
    fi

    # 6b. Run bridge (it places the binary at $XFPM_BIN/xfpm)
    run_bridge "$bridge_path" "$runtime"
  else
    # 6c. No JS runtime — direct download fallback
    fallback_direct_download
  fi

  # 7. Verify binary
  info "Verifying binary..."
  local installed_version
  installed_version="$(verify_binary)"
  chmod +x "$XFPM_BIN/$TOOL_NAME"

  # 8. Symlink aliases
  install_aliases
  chmod 755 "$XFPM_BIN/$TOOL_NAME"
  chmod -R a+rX "$XFPM_BIN" 2>/dev/null || true

  # 9. Update PATH in shell profiles
  add_to_path

  # 10. Install system symlinks into /usr/local/bin (enables 'sudo xfpm')
  install_system_symlinks

  # 11. Check for PATH conflict
  if command -v "$TOOL_NAME" >/dev/null 2>&1; then
    local found_path
    found_path="$(command -v "$TOOL_NAME")"
    # Resolve any symlinks to compare real paths
    local real_found
    real_found="$(readlink -f "$found_path" 2>/dev/null || echo "$found_path")"
    local real_current="$XFPM_BIN/$TOOL_NAME"
    
    if [ "$real_found" != "$real_current" ]; then
      if [ -w "$found_path" ] && [ -w "$(dirname "$found_path")" ]; then
        info "Updating shadowed binary at $found_path with a symlink to $real_current..."
        ln -sf "$real_current" "$found_path"
        ok "Resolved: $found_path now points to $real_current"
      else
        echo ""
        warn "⚠️  ${C_BOLD}SHADOWING DETECTED!${C_RESET}"
        warn "Another version of '$TOOL_NAME' is active at: ${C_CYAN}$found_path${C_RESET}"
        warn "This old version will take precedence over the one just installed."
        warn ""
        warn "To use the new version (v$VERSION), please remove the old one:"
        warn "  ${C_BOLD}sudo rm $found_path${C_RESET}"
        echo ""
      fi
    fi
  fi

  # 12. Deploy native XRU engine & stdlib by default
  install_xru false

  # ─── Success ──────────────────────────────────────────────
  echo ""
  echo -e "${C_GREEN}${C_BOLD}✓ XFPM ${installed_version} installed successfully!${C_RESET}"
  echo -e "  Binary : ${C_CYAN}$XFPM_BIN/$TOOL_NAME${C_RESET}"
  echo -e "  XRU    : ${C_CYAN}$XFPM_BIN/xru${C_RESET} ${C_DIM}(stdlib: $XFPM_ROOT/lib/xru/std)${C_RESET}"
  echo -e "  Aliases: ${C_DIM}xyp, xypcli, xfpmc (create), xfpmx (exec)${C_RESET}"
  if [ -L "/usr/local/bin/$TOOL_NAME" ]; then
    echo -e "  System : ${C_GREEN}Available system-wide and with sudo (/usr/local/bin/$TOOL_NAME)${C_RESET}"
  fi
  echo ""
  echo -e "  To get started, restart your terminal and type:"
  echo -e "  ${C_BOLD}xfpm --help${C_RESET}"
  echo -e "  ${C_BOLD}xru --help${C_RESET}"
  echo ""
}

# ─────────────────────────────────────────────────────────────
# XRU (XyPriss Rule Unit) & Stdlib installer flow
# ─────────────────────────────────────────────────────────────
XRU_REPO="Nehonix-Team/xru"

install_xru() {
  local is_standalone="${1:-true}"
  local xfpm_lib="$XFPM_ROOT/lib/xru/std"

  if [ "$is_standalone" = "true" ]; then
    echo ""
    echo -e "${C_BOLD}${C_CYAN}XRU Standalone Installer${C_RESET}"
    echo -e "${C_CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${C_RESET}"
    detect_platform
    info "Platform  : ${OS_NAME}/${ARCH_NAME}"
    check_downloader
    info "Downloader: $DOWNLOADER"
  else
    info "Deploying native XRU engine & standard library..."
  fi

  mkdir -p "$XFPM_BIN" "$xfpm_lib"
  chmod 755 "$XFPM_ROOT" "$XFPM_BIN" 2>/dev/null || true

  local xru_ver="${XRU_VERSION:-latest}"
  if [ "$xru_ver" = "latest" ]; then
    local tag
    tag=$(curl -s "https://api.github.com/repos/${XRU_REPO}/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)
    if [ -n "$tag" ]; then
      xru_ver="${tag#v}"
    else
      xru_ver="2.0.0"
    fi
  else
    xru_ver="${xru_ver#v}"
  fi
  local xru_tar="xru-v${xru_ver}-${OS_NAME}-${ARCH_NAME}.tar.gz"
  local xru_url="https://github.com/${XRU_REPO}/releases/download/v${xru_ver}/${xru_tar}"
  local tmp_archive
  tmp_archive="$(mktemp "${TMPDIR:-/tmp}/xru_XXXXXX.tar.gz")"
  local tmp_extract
  tmp_extract="$(mktemp -d "${TMPDIR:-/tmp}/xru_ext_XXXXXX")"
  local dest_bin="$XFPM_BIN/xru"

  info "Downloading XRU bundle from: $xru_url"
  if download "$xru_url" "$tmp_archive" 2>/dev/null; then
    tar -xzf "$tmp_archive" -C "$tmp_extract" 2>/dev/null || true
  fi

  if [ -f "$tmp_extract/bin/xru" ]; then
    mv -f "$tmp_extract/bin/xru" "$dest_bin"
    chmod 755 "$dest_bin"
    if [ -d "$tmp_extract/std" ]; then
      rm -rf "$xfpm_lib"
      mkdir -p "$xfpm_lib"
      cp -r "$tmp_extract/std/"* "$xfpm_lib/"
    fi
  else
    # Fallback to direct raw binary
    local xru_raw_bin="xru-${OS_NAME}-${ARCH_NAME}"
    local xru_raw_url="https://github.com/${XRU_REPO}/releases/latest/download/${xru_raw_bin}"
    download "$xru_raw_url" "$dest_bin" 2>/dev/null || true
    chmod 755 "$dest_bin" 2>/dev/null || true
  fi

  # Clean legacy binary paths to avoid PATH precedence conflicts
  local legacy_candidates=(
    "/usr/local/bin/xru"
    "$HOME/.local/bin/xru"
    "$HOME/.xru/bin/xru"
  )
  for legacy in "${legacy_candidates[@]}"; do
    if [ -e "$legacy" ] || [ -L "$legacy" ]; then
      if [ -L "$legacy" ] && [ "$(readlink "$legacy" 2>/dev/null || true)" = "$dest_bin" ]; then
        continue
      fi
      if [ -w "$legacy" ] || [ -w "$(dirname "$legacy" 2>/dev/null || true)" ]; then
        rm -f "$legacy" 2>/dev/null || true
      elif command -v sudo >/dev/null 2>&1 && [ "${EUID:-$(id -u)}" -ne 0 ]; then
        warn "Removing conflicting legacy binary at $legacy..."
        sudo rm -f "$legacy" 2>/dev/null || true
      fi
    fi
  done

  rm -rf "$tmp_archive" "$tmp_extract" 2>/dev/null || true

  # Ensure /usr/local/bin/xru points to the fresh installation if writable
  if [ -w /usr/local/bin ]; then
    rm -f /usr/local/bin/xru 2>/dev/null || true
    ln -sf "$dest_bin" "/usr/local/bin/xru" 2>/dev/null || true
  fi

  if [ "$is_standalone" = "true" ]; then
    add_to_path
    echo ""
    echo -e "${C_GREEN}${C_BOLD}✓ XRU installed successfully!${C_RESET}"
    echo -e "  Binary : ${C_CYAN}$dest_bin${C_RESET}"
    echo -e "  Stdlib : ${C_CYAN}$xfpm_lib${C_RESET}"
    echo -e "  Usage  : ${C_YELLOW}xru <script.xru>${C_RESET}"
    echo ""
  else
    ok "XRU engine & standard library installed at: ${C_CYAN}$xfpm_lib${C_RESET}"
  fi
}

# ─────────────────────────────────────────────────────────────
# Entry point
# ─────────────────────────────────────────────────────────────
case "$ACTION" in
  uninstall) uninstall_xfpm ;;
  xru)       install_xru true ;;
  *)         install_xfpm   ;;
esac
