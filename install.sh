#!/usr/bin/env bash

# php-call-graph-viz installation script
# Usage: curl -sSL https://raw.githubusercontent.com/hightemp/php-call-graph-viz/main/install.sh | bash

set -e

REPO="hightemp/php-call-graph-viz"
BINARY_NAME="php-call-graph-viz"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

detect_os() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$OS" in
        linux*)  echo "linux" ;;
        darwin*) echo "darwin" ;;
        msys*|mingw*|cygwin*) echo "windows" ;;
        *) log_error "Unsupported OS: $OS"; exit 1 ;;
    esac
}

detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)   echo "x86_64" ;;
        aarch64|arm64)  echo "arm64" ;;
        *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
}

get_latest_release() {
    curl -s "https://api.github.com/repos/$REPO/releases/latest" |
        grep '"tag_name":' |
        sed -E 's/.*"([^"]+)".*/\1/'
}

download_and_install() {
    OS=$(detect_os)
    ARCH=$(detect_arch)
    VERSION=$(get_latest_release)

    if [ -z "$VERSION" ]; then
        log_error "Failed to get latest release version"
        exit 1
    fi

    log_info "Detected OS: $OS, Architecture: $ARCH"
    log_info "Latest version: $VERSION"

    # Construct download URL
    if [ "$OS" = "windows" ]; then
        ARCHIVE_NAME="${BINARY_NAME}_${VERSION#v}_Windows_${ARCH}.zip"
        ARCHIVE_TYPE="zip"
    else
        OS_TITLE="$(echo "$OS" | sed 's/./\U&/')" # Capitalize first letter
        ARCHIVE_NAME="${BINARY_NAME}_${VERSION#v}_${OS_TITLE}_${ARCH}.tar.gz"
        ARCHIVE_TYPE="tar.gz"
    fi

    DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE_NAME"
    
    log_info "Downloading $ARCHIVE_NAME..."
    
    TMP_DIR=$(mktemp -d)
    trap "rm -rf $TMP_DIR" EXIT
    
    if ! curl -sL "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE_NAME"; then
        log_error "Failed to download $DOWNLOAD_URL"
        exit 1
    fi

    log_info "Extracting archive..."
    
    if [ "$ARCHIVE_TYPE" = "zip" ]; then
        unzip -q "$TMP_DIR/$ARCHIVE_NAME" -d "$TMP_DIR"
    else
        tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"
    fi

    # Create install directory if it doesn't exist
    mkdir -p "$INSTALL_DIR"

    # Install binary
    BINARY_PATH="$TMP_DIR/$BINARY_NAME"
    if [ "$OS" = "windows" ]; then
        BINARY_PATH="$BINARY_PATH.exe"
    fi

    if [ ! -f "$BINARY_PATH" ]; then
        log_error "Binary not found in archive"
        exit 1
    fi

    log_info "Installing to $INSTALL_DIR/$BINARY_NAME..."
    
    cp "$BINARY_PATH" "$INSTALL_DIR/$BINARY_NAME"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"

    log_info "✓ Successfully installed $BINARY_NAME $VERSION"
    
    # Check if install directory is in PATH
    if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        log_warn "$INSTALL_DIR is not in your PATH"
        log_warn "Add this line to your ~/.bashrc or ~/.zshrc:"
        echo ""
        echo "    export PATH=\"\$PATH:$INSTALL_DIR\""
        echo ""
    else
        log_info "Run '$BINARY_NAME --help' to get started"
    fi
}

main() {
    log_info "Installing $BINARY_NAME..."
    
    # Check dependencies
    for cmd in curl tar; do
        if ! command -v $cmd &> /dev/null; then
            log_error "$cmd is required but not installed"
            exit 1
        fi
    done

    download_and_install
}

main
