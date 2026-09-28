#!/usr/bin/env bash
set -euo pipefail

APP_NAME="LanFileTransfer-Go"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${PROJECT_ROOT}/build/bin"
PLATFORM=""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m'

print_step() { echo -e "${YELLOW}--- $1 ---${NC}"; }
print_ok()   { echo -e "${GREEN}[OK]${NC} $1"; }
print_err()  { echo -e "${RED}[ERROR]${NC} $1"; }
print_info() { echo -e "${CYAN}[INFO]${NC} $1"; }
print_skip() { echo -e "${YELLOW}[SKIP]${NC} $1"; }
print_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  --dev             Build in development mode (debug tools enabled)
  --clean           Clean build directory before building
  --skip-frontend   Skip frontend dependency installation
  --skip-tests      Skip running Go tests before build
  --platform <str>  Cross-compile target (e.g.: windows/amd64, linux/amd64, darwin/amd64)
  --help            Show this help message

Examples:
  ./scripts/build.sh                          # Production build
  ./scripts/build.sh --dev                    # Development build
  ./scripts/build.sh --clean --skip-tests     # Clean build, skip tests
  ./scripts/build.sh --platform windows/amd64 # Cross-compile for Windows
EOF
    exit 0
}

DEV_MODE=false
CLEAN=false
SKIP_FRONTEND=false
SKIP_TESTS=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dev)             DEV_MODE=true; shift ;;
        --clean)           CLEAN=true; shift ;;
        --skip-frontend)   SKIP_FRONTEND=true; shift ;;
        --skip-tests)      SKIP_TESTS=true; shift ;;
        --platform)        PLATFORM="$2"; shift 2 ;;
        --help)            usage ;;
        *)                 echo "Unknown option: $1"; usage ;;
    esac
done

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  ${APP_NAME} Build Script${NC}"
echo -e "${CYAN}========================================${NC}"
echo ""

check_command() {
    local name="$1"
    local hint="${2:-}"
    if ! command -v "$name" &>/dev/null; then
        print_err "'$name' not found!"
        [ -n "$hint" ] && echo -e "${YELLOW}  Hint: $hint${NC}"
        exit 1
    fi
    local version
    if [ "$name" = "go" ]; then
        version=$("$name" version 2>&1 | head -n1)
    else
        version=$("$name" --version 2>&1 | head -n1)
    fi
    print_ok "$name found: $version"
}

check_prerequisites() {
    print_step "Checking prerequisites"

    check_command "go" "Install Go 1.23+: https://go.dev/dl/"

    local go_version
    go_version=$(go version | sed -n 's/.*\(go[0-9]\+\.[0-9]\+\).*/\1/p')
    if [[ "$go_version" < "go1.23" ]]; then
        print_warn "Go version may be too old. Recommended: 1.23+"
    fi

    check_command "node" "Install Node.js 18+: https://nodejs.org/"
    check_command "npm" "Node.js should include npm"

    if ! command -v "wails" &>/dev/null; then
        print_info "Wails CLI not found, installing..."
        go install github.com/wailsapp/wails/v2/cmd/wails@latest
        local gopath
        gopath=$(go env GOPATH)
        export PATH="$gopath/bin:$PATH"
        if ! command -v "wails" &>/dev/null; then
            print_err "Wails CLI installation failed. Add GOPATH/bin to PATH and retry."
            exit 1
        fi
    fi
    print_ok "Wails CLI: $(wails version)"
    echo ""
}

install_frontend_deps() {
    if $SKIP_FRONTEND; then
        print_skip "Frontend dependency installation skipped"
        return
    fi
    print_step "Installing frontend dependencies"
    local frontend_dir="${PROJECT_ROOT}/frontend"
    if [ ! -d "${frontend_dir}/node_modules" ]; then
        pushd "$frontend_dir" >/dev/null
        npm install
        popd >/dev/null
        print_ok "Frontend dependencies installed"
    else
        print_ok "Frontend dependencies already installed"
    fi
    echo ""
}

run_tests() {
    if $SKIP_TESTS; then
        print_skip "Tests skipped"
        return
    fi
    print_step "Running tests"
    pushd "$PROJECT_ROOT" >/dev/null
    if ! go test ./... -count=1; then
        print_err "Tests failed!"
        popd >/dev/null
        exit 1
    fi
    print_ok "All tests passed"
    popd >/dev/null
    echo ""
}

clean_build() {
    print_step "Cleaning build directory"
    if [ -d "$BUILD_DIR" ]; then
        rm -rf "${BUILD_DIR:?}"/*
        print_ok "Build directory cleaned"
    else
        mkdir -p "$BUILD_DIR"
        print_ok "Build directory created"
    fi
    echo ""
}

resolve_platform() {
    if [ -z "$PLATFORM" ]; then
        return
    fi

    local goos="${PLATFORM%/*}"
    local goarch="${PLATFORM#*/}"

    if [ "$goos" = "$PLATFORM" ] || [ -z "$goarch" ]; then
        print_err "Invalid platform format. Use: os/arch (e.g.: windows/amd64)"
        exit 1
    fi

    case "$goos" in
        windows) APP_NAME="LanFileTransfer-Go.exe" ;;
        linux)   APP_NAME="LanFileTransfer-Go" ;;
        darwin)  APP_NAME="LanFileTransfer-Go" ;;
        *)
            print_err "Unsupported OS: $goos"
            exit 1
            ;;
    esac

    print_info "Cross-compiling for: $goos/$goarch"
    echo ""
}

build_app() {
    print_step "Building application"

    pushd "$PROJECT_ROOT" >/dev/null

    local build_args=("build")

    if $DEV_MODE; then
        build_args+=("-debug")
        print_info "Build mode: dev (debug enabled)"
    else
        print_info "Build mode: production"
    fi

    if [ -n "$PLATFORM" ]; then
        local goos="${PLATFORM%/*}"
        if [ "$goos" = "windows" ]; then
            build_args+=("-tags" "native_webview2loader")
        fi
        export GOOS="$goos"
        export GOARCH="${PLATFORM#*/}"
    fi

    local output_path="${BUILD_DIR}/${APP_NAME}"
    build_args+=("-o" "$output_path")

    wails "${build_args[@]}"

    local output_path="${BUILD_DIR}/${APP_NAME}"
    local app_bundle="${BUILD_DIR}/${APP_NAME}.app"

    if [ -f "$output_path" ]; then
        local size
        if [[ "$(uname)" == "Darwin" ]]; then
            size=$(stat -f%z "$output_path" | awk '{printf "%.2f MB", $1/1048576}')
        else
            size=$(du -h "$output_path" | cut -f1)
        fi
        echo ""
        print_ok "Build successful!"
        echo -e "${WHITE}  Output: ${output_path}${NC}"
        echo -e "${WHITE}  Size:   ${size}${NC}"
    elif [ -d "$app_bundle" ]; then
        local binary_path="${app_bundle}/Contents/MacOS/${APP_NAME}"
        local size
        if [[ "$(uname)" == "Darwin" ]]; then
            size=$(du -sh "$app_bundle" | cut -f1)
        else
            size=$(du -sh "$app_bundle" | cut -f1)
        fi
        echo ""
        print_ok "Build successful!"
        echo -e "${WHITE}  Output: ${app_bundle}${NC}"
        echo -e "${WHITE}  Size:   ${size}${NC}"
        if [ -f "$binary_path" ]; then
            echo -e "${WHITE}  Binary: ${binary_path}${NC}"
        fi
    else
        print_err "Output file not found: $output_path"
        popd >/dev/null
        exit 1
    fi

    popd >/dev/null
    echo ""
}

check_prerequisites
install_frontend_deps
run_tests

if $CLEAN; then
    clean_build
fi

resolve_platform
build_app

echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  Build completed!${NC}"
echo -e "${CYAN}========================================${NC}"