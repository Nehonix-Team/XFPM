#!/bin/bash
# ***************************************************************************
# * XFPM (XyPriss Fast Package Manager)
# * Cross-Platform Build Architecture & Anti-Reverse Engineering Pipeline
# * @license Nehonix OSL (NOSL)
# ***************************************************************************

set -e

# Colors for premium terminal output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Configuration
DIST_DIR="dist"
BINARY_NAME="xfpm"
ENTRY_POINT="./cmd/xfpm"
# Extract version from Go source
VERSION=$(grep "BinVersion =" internal/utils/lib_version.go | cut -d'"' -f2)

echo -e "\n${BLUE}${BOLD} 🚀 XFPM SECURE BUILD ENGINE ${NC} ${DIM}${VERSION}${NC}"
echo -e "${DIM} ──────────────────────────────────────────────────${NC}\n"

# Flag parsing
COMPRESS=false
CURRENT_ONLY=false
NO_OBFUSCATE=false

for arg in "$@"; do
    case "$arg" in
        --compress)
            COMPRESS=true
            ;;
        --current|-c)
            CURRENT_ONLY=true
            ;;
        --no-obfuscate)
            NO_OBFUSCATE=true
            ;;
    esac
done

if [ "$COMPRESS" = true ] && ! command -v upx >/dev/null 2>&1; then
    echo -e " ${YELLOW}⚠️  UPX not found. Compression will be skipped.${NC}"
    COMPRESS=false
fi

# Detect Obfuscator (Garble) & Toolchain
USE_GARBLE=false
GARBLE_BIN=""

if [ "$NO_OBFUSCATE" = false ]; then
    # Ensure standalone toolchain is used if present to allow garble AST overlays
    if [ -d "$HOME/.local/go1.27.1" ]; then
        export GOROOT="$HOME/.local/go1.27.1"
        export PATH="$HOME/.local/go1.27.1/bin:$HOME/go/bin:$PATH"
    fi

    if command -v garble >/dev/null 2>&1; then
        GARBLE_BIN="garble"
        USE_GARBLE=true
    elif [ -x "$HOME/go/bin/garble" ]; then
        GARBLE_BIN="$HOME/go/bin/garble"
        USE_GARBLE=true
    fi
fi

if [ "$USE_GARBLE" = true ]; then
    echo -e " ${GREEN}🔒 Anti-Reverse Engineering: Active${NC} ${DIM}(Garble AST literal encryption & symbol hashing)${NC}"
else
    echo -e " ${YELLOW}ℹ️  Standard Compiler Mode${NC} ${DIM}(Enhanced trimpath & metadata stripping)${NC}"
fi

# Multi-platform targets
if [ "$CURRENT_ONLY" = true ]; then
    HOST_OS=$(go env GOOS)
    HOST_ARCH=$(go env GOARCH)
    TARGETS=("${HOST_OS}/${HOST_ARCH}")
    echo -e " ${CYAN}🎯 Building target:${NC} ${BOLD}${HOST_OS}/${HOST_ARCH}${NC} ${DIM}(current platform)${NC}\n"
else
    TARGETS=(
        "linux/amd64"
        "linux/arm64"
        "darwin/amd64"
        "darwin/arm64"
        "windows/amd64"
        "windows/arm64"
    )
    echo -e " ${CYAN}🎯 Building all 6 multi-platform targets...${NC}\n"
fi

# Function to scrub embedded Go buildinfo / module dependency tables (path\t, mod\t, dep\t)
strip_go_buildinfo() {
    local target_file="$1"
    python3 -c "
import sys, re
target = sys.argv[1]
try:
    with open(target, 'r+b') as f:
        data = bytearray(f.read())
        changed = False
        # 1. Neutralize magic header '\xff Go buildinf:'
        for m in re.finditer(b'\\xff Go buildinf:', data):
            data[m.start():m.start()+32] = b'\x00' * 32
            changed = True
        # 2. Neutralize embedded module metadata tables (path\t, mod\t, dep\t)
        for m in re.finditer(b'path\\t', data):
            end = data.find(b'\x00', m.start())
            if end != -1:
                data[m.start():end] = b'\x00' * (end - m.start())
                changed = True
        if changed:
            f.seek(0)
            f.write(data)
            f.truncate()
except Exception:
    pass
" "$target_file" 2>/dev/null || true
}

# Initialization
mkdir -p "$DIST_DIR"

# Common hardened Go flags
# -trimpath: removes host filesystem paths (/home/user/...)
# -buildvcs=false: removes VCS metadata (commit, dirty status)
# -ldflags="-s -w -buildid=": strips symbol table, DWARF debug info, and build id
GO_FLAGS=(-trimpath -buildvcs=false -ldflags="-s -w -buildid=")

# Build Loop
for TARGET in "${TARGETS[@]}"; do
    IFS="/" read -r OS ARCH <<< "$TARGET"
    
    OUTPUT="${BINARY_NAME}-${OS}-${ARCH}"
    if [ "$OS" == "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi

    printf "  ${CYAN}%-20s${NC} ${DIM}»${NC} " "Creating ${OS}/${ARCH}"
    
    TARGET_PATH="${DIST_DIR}/${OUTPUT}"
    rm -f "$TARGET_PATH"

    if [ "$USE_GARBLE" = true ]; then
        # Obfuscate AST, symbols, types, and string literals
        if GOOS=$OS GOARCH=$ARCH "$GARBLE_BIN" -literals -tiny build "${GO_FLAGS[@]}" -o "$TARGET_PATH" "$ENTRY_POINT"; then
            strip_go_buildinfo "$TARGET_PATH"
            echo -e "${GREEN}✓ OBFUSCATED${NC}"
        else
            echo -e "${NC}❌ FAILED (Garble)${NC}"
            exit 1
        fi
    else
        # Standard hardened build
        if GOOS=$OS GOARCH=$ARCH go build "${GO_FLAGS[@]}" -o "$TARGET_PATH" "$ENTRY_POINT"; then
            strip_go_buildinfo "$TARGET_PATH"
            echo -e "${GREEN}✓ STRIPPED${NC}"
        else
            echo -e "${NC}❌ FAILED (Go)${NC}"
            exit 1
        fi
    fi
        
    # Compression phase (optional UPX)
    if [ "$COMPRESS" = true ]; then
        printf "  ${MAGENTA}%-20s${NC} ${DIM}»${NC} " "UPX Compressing"
        if upx --best "$TARGET_PATH" > /dev/null 2>&1; then
            echo -e "${MAGENTA}✨ OPTIMIZED${NC}"
        else
            echo -e "${YELLOW}⨯ SKIPPED${NC}"
        fi
    fi

    # If building for host, update local root ./xfpm and ~/.xfpm/bin/xfpm
    if [ "$OS" = "$(go env GOOS)" ] && [ "$ARCH" = "$(go env GOARCH)" ]; then
        cp "$TARGET_PATH" "./${BINARY_NAME}"
        chmod +x "./${BINARY_NAME}"
        if [ -d "$HOME/.xfpm/bin" ]; then
            cp -f "$TARGET_PATH" "$HOME/.xfpm/bin/${BINARY_NAME}"
            chmod +x "$HOME/.xfpm/bin/${BINARY_NAME}"
        fi
    fi
done

echo -e "\n${GREEN}${BOLD} ✅ BUILD PIPELINE COMPLETE ${NC}"
echo -e "${DIM} Artifacts stored in: ./${DIST_DIR}${NC}\n"

ls -lh "$DIST_DIR" | grep "$BINARY_NAME" | awk '{print "  " $5 "\t" $9}'
echo ""
