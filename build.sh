#!/usr/bin/env bash
# ==============================================================================
# ado-board-cli Build Script (Linux amd64 & Windows amd64)
# ==============================================================================
set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

BINARY_NAME="ado-board-cli"
PKG_DIR="."
BUILD_DIR="bin"
BUILD_TARGET="host" # host, all, linux, windows, release
SKIP_TESTS=false
CLEAN=false

for arg in "$@"; do
    case "$arg" in
        --release|-r)
            BUILD_TARGET="release"
            ;;
        --all|-a|--cross)
            BUILD_TARGET="all"
            ;;
        --windows|-w|--win)
            BUILD_TARGET="windows"
            ;;
        --linux|-l)
            BUILD_TARGET="linux"
            ;;
        --clean|-c)
            CLEAN=true
            ;;
        --skip-tests|-s)
            SKIP_TESTS=true
            ;;
        --help|-h)
            echo "Usage: ./build.sh [options]"
            echo "Options:"
            echo "  -r, --release        Build release binaries & archives (.tar.xz for Linux, .zip for Windows)"
            echo "  -a, --all, --cross   Build both binaries (./bin/ado-board-cli & ./bin/ado-board-cli.exe)"
            echo "  -l, --linux          Build Linux amd64 binary (./bin/ado-board-cli)"
            echo "  -w, --windows        Build Windows amd64 binary with icon (./bin/ado-board-cli.exe)"
            echo "  -s, --skip-tests     Skip running unit tests"
            echo "  -c, --clean          Clean build artifacts and temporary resource files"
            echo "  -h, --help           Show this help message"
            exit 0
            ;;
    esac
done

if [ "$CLEAN" = true ]; then
    echo -e "${BLUE}==> Cleaning build artifacts and resource files...${NC}"
    rm -rf bin/ builds/ dist/ build/ ado-board-cli ado-board-cli.exe *.syso assets/icon.ico versioninfo.json
    echo -e "${GREEN}✓ Clean completed.${NC}"
    exit 0
fi

echo -e "${BLUE}==> [1/4] Checking Go environment...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go compiler is not installed or not found in PATH.${NC}"
    exit 1
fi
go version

VERSION=$(sed -n 's/.*Version = "\(.*\)".*/\1/p' pkg/version/version.go 2>/dev/null || echo "0.05")
if [ -z "$VERSION" ]; then
    VERSION="0.05"
fi
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-s -w -X ado-board-cli/pkg/version.GitCommit=${GIT_COMMIT} -X ado-board-cli/pkg/version.BuildDate=${BUILD_DATE}"

echo -e "  • Building ${CYAN}${BINARY_NAME}${NC} version ${GREEN}v${VERSION}${NC} (${GIT_COMMIT:-dev})"

if [ "$SKIP_TESTS" = false ]; then
    echo -e "${BLUE}==> [2/4] Running tests and verification...${NC}"
    go test ./...
else
    echo -e "${YELLOW}==> [2/4] Skipping tests (-s/--skip-tests)...${NC}"
fi

mkdir -p "${BUILD_DIR}"

ensure_windows_resources() {
    echo -e "${BLUE}==> Generating Windows icon and PE resource metadata from assets/icon.svg...${NC}"
    if [ ! -f "assets/icon.svg" ]; then
        echo -e "${RED}Error: Source icon vector missing at: assets/icon.svg${NC}"
        exit 1
    fi

    echo -e "  • Rendering multi-resolution icon (assets/icon.ico) from assets/icon.svg..."
    go run scripts/genicon.go

    local V_MAJ V_MIN V_PATCH
    IFS='.' read -r V_MAJ V_MIN V_PATCH _ <<< "${VERSION}.0.0.0"
    V_MAJ=$((10#${V_MAJ:-0}))
    V_MIN=$((10#${V_MIN:-0}))
    V_PATCH=$((10#${V_PATCH:-0}))
    local PE_VERSION="${V_MAJ}.${V_MIN}.${V_PATCH}.0"

    echo -e "  • Generating versioninfo.json for version ${VERSION} (${PE_VERSION})..."
    cat <<EOF > versioninfo.json
{
  "FixedFileInfo": {
    "FileVersion": {
      "Major": ${V_MAJ},
      "Minor": ${V_MIN},
      "Patch": ${V_PATCH},
      "Build": 0
    },
    "ProductVersion": {
      "Major": ${V_MAJ},
      "Minor": ${V_MIN},
      "Patch": ${V_PATCH},
      "Build": 0
    },
    "FileFlagsMask": "3f",
    "FileFlags": "00",
    "FileOS": "040004",
    "FileType": "01",
    "FileSubType": "00"
  },
  "StringFileInfo": {
    "Comments": "Azure DevOps Work Item Tree and Board CLI",
    "CompanyName": "Jagadeesh Kotra",
    "FileDescription": "Azure DevOps Work Item Tree and Board CLI",
    "FileVersion": "${PE_VERSION}",
    "InternalName": "ado-board-cli",
    "LegalCopyright": "Copyright (c) 2026 Jagadeesh Kotra",
    "OriginalFilename": "ado-board-cli.exe",
    "ProductName": "Azure DevOps Board CLI",
    "ProductVersion": "${PE_VERSION}"
  },
  "VarFileInfo": {
    "Translation": {
      "LangID": "0409",
      "CharsetID": "04B0"
    }
  },
  "IconPath": "assets/icon.ico",
  "ManifestPath": ""
}
EOF

    GOVERSIONINFO="goversioninfo"
    if ! command -v goversioninfo &> /dev/null; then
        GOPATH_BIN="$(go env GOPATH)/bin"
        if [ -x "${GOPATH_BIN}/goversioninfo" ]; then
            GOVERSIONINFO="${GOPATH_BIN}/goversioninfo"
        else
            echo -e "  • Installing goversioninfo resource compiler..."
            go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
            GOVERSIONINFO="${GOPATH_BIN}/goversioninfo"
        fi
    fi

    echo -e "  • Compiling Windows resource_windows_amd64.syso via goversioninfo..."
    "${GOVERSIONINFO}" -64 -o resource_windows_amd64.syso versioninfo.json
    echo -e "${GREEN}✓ Generated resource_windows_amd64.syso (Windows PE icon and metadata ready)${NC}"
}

build_linux() {
    echo -e "  • Compiling Linux amd64 -> ${GREEN}${BUILD_DIR}/${BINARY_NAME}${NC}..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "${BUILD_DIR}/${BINARY_NAME}" "${PKG_DIR}"
    if command -v strip &> /dev/null; then
        strip --strip-all "${BUILD_DIR}/${BINARY_NAME}" 2>/dev/null || strip "${BUILD_DIR}/${BINARY_NAME}" 2>/dev/null || true
    fi
}

build_windows() {
    ensure_windows_resources
    echo -e "  • Compiling Windows amd64 -> ${GREEN}${BUILD_DIR}/${BINARY_NAME}.exe${NC}..."
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="${LDFLAGS}" -o "${BUILD_DIR}/${BINARY_NAME}.exe" "${PKG_DIR}"
}

package_linux() {
    local ARCHIVE_NAME="${BINARY_NAME}-v${VERSION}-linux-amd64.tar.xz"
    echo -e "  • Packaging Linux archive -> ${GREEN}${BUILD_DIR}/${ARCHIVE_NAME}${NC}..."
    tar -cJf "${BUILD_DIR}/${ARCHIVE_NAME}" -C "${BUILD_DIR}" "${BINARY_NAME}"
    echo -e "${GREEN}✓ Created ${BUILD_DIR}/${ARCHIVE_NAME}${NC}"
}

package_windows() {
    local ARCHIVE_NAME="${BINARY_NAME}-v${VERSION}-windows-amd64.zip"
    echo -e "  • Packaging Windows archive -> ${GREEN}${BUILD_DIR}/${ARCHIVE_NAME}${NC}..."
    if command -v zip &> /dev/null; then
        (cd "${BUILD_DIR}" && zip -9 -q "${ARCHIVE_NAME}" "${BINARY_NAME}.exe")
    elif command -v python3 &> /dev/null; then
        (cd "${BUILD_DIR}" && python3 -m zipfile -c "${ARCHIVE_NAME}" "${BINARY_NAME}.exe")
    else
        go run - <<GOEOF
package main
import (
    "archive/zip"
    "io"
    "os"
)
func main() {
    zf, err := os.Create("${BUILD_DIR}/${ARCHIVE_NAME}")
    if err != nil { panic(err) }
    defer zf.Close()
    zw := zip.NewWriter(zf)
    defer zw.Close()
    src, err := os.Open("${BUILD_DIR}/${BINARY_NAME}.exe")
    if err != nil { panic(err) }
    defer src.Close()
    w, err := zw.Create("${BINARY_NAME}.exe")
    if err != nil { panic(err) }
    io.Copy(w, src)
}
GOEOF
    fi
    echo -e "${GREEN}✓ Created ${BUILD_DIR}/${ARCHIVE_NAME}${NC}"
}

case "$BUILD_TARGET" in
    release)
        echo -e "${CYAN}==> [3/4] Building release binaries for Linux & Windows (v${VERSION})...${NC}"
        build_linux
        build_windows
        echo -e "\n${CYAN}==> [4/4] Creating distribution release archives in ${BUILD_DIR}/...${NC}"
        package_linux
        package_windows
        echo -e "\n${BLUE}==> Release summary:${NC}"
        ls -lh "${BUILD_DIR}"/ado-board-cli*
        echo -e "\n${GREEN}✓ Release builds and archives successfully created in ./${BUILD_DIR}/${NC}"
        ;;
    all)
        echo -e "${CYAN}==> [3/4] Building binaries for Linux amd64 & Windows amd64 in ${BUILD_DIR}/...${NC}"
        build_linux
        build_windows
        echo -e "\n${BLUE}==> [4/4] Build summary:${NC}"
        ls -lh "${BUILD_DIR}"/ado-board-cli*
        echo -e "\n${GREEN}✓ Standalone binaries ready in ./${BUILD_DIR}/${NC}"
        ;;
    windows)
        echo -e "${CYAN}==> [3/4] Building Windows amd64 binary (with icon & PE metadata)...${NC}"
        build_windows
        echo -e "\n${BLUE}==> [4/4] Verifying binary...${NC}"
        SIZE=$(du -h "./${BUILD_DIR}/${BINARY_NAME}.exe" | cut -f1)
        echo -e "${GREEN}✓ Build succeeded! Binary created at: ./${BUILD_DIR}/${BINARY_NAME}.exe (${SIZE})${NC}"
        ;;
    linux|host)
        echo -e "${BLUE}==> [3/4] Building Linux amd64 binary ${BUILD_DIR}/${BINARY_NAME}...${NC}"
        build_linux
        echo -e "\n${BLUE}==> [4/4] Verifying binary...${NC}"
        if [ -f "./${BUILD_DIR}/${BINARY_NAME}" ]; then
            SIZE=$(du -h "./${BUILD_DIR}/${BINARY_NAME}" | cut -f1)
            echo -e "${GREEN}✓ Build succeeded! Binary created at: ./${BUILD_DIR}/${BINARY_NAME} (${SIZE})${NC}\n"
            echo -e "${YELLOW}Quick Start & Setup:${NC}"
            echo -e "  1. Launch the interactive TUI:"
            echo -e "     ${GREEN}./${BUILD_DIR}/${BINARY_NAME}${NC}"
            echo -e "  2. Press ${BLUE}Shift+S${NC} (or ${BLUE}S${NC}) to open ${BLUE}Settings${NC} and configure:"
            echo -e "     • Azure DevOps Personal Access Token (PAT)"
            echo -e "     • Google Gemini API Key (optional, for AI dock)"
            echo -e "     • Default Organization, Project & preferences"
            echo -e ""
            echo -e "${YELLOW}Target Builds & Releases:${NC}"
            echo -e "  • ${CYAN}./build.sh --release${NC} Build Linux .tar.xz and Windows .zip release packages"
            echo -e "  • ${CYAN}./build.sh --all${NC}     Build Linux and Windows binaries in ./${BUILD_DIR}/"
            echo -e "  • ${CYAN}./build.sh --windows${NC} Build Windows amd64 with icon (${BUILD_DIR}/ado-board-cli.exe)"
        else
            echo -e "${RED}Build failed: binary was not generated.${NC}"
            exit 1
        fi
        ;;
esac
