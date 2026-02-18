#!/bin/sh
# Install script for Avalanche Platform CLI
# Usage: curl -sSfL https://raw.githubusercontent.com/ava-labs/platform-cli/main/install.sh | sh
#        curl -sSfL ... | sh -s -- -b /custom/path
#        curl -sSfL ... | sh -s -- -v v0.2.0

set -e

REPO="ava-labs/platform-cli"
BINARY="platform"
GITHUB="https://github.com"

# Defaults
INSTALL_DIR="/usr/local/bin"
VERSION=""

usage() {
    cat <<EOF
Usage: install.sh [-b install_dir] [-v version]

Options:
  -b DIR      Install directory (default: /usr/local/bin, falls back to ~/.local/bin)
  -v VERSION  Specific version to install (e.g., v0.1.0). Defaults to latest release.
  -h          Show this help message
EOF
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            -b) INSTALL_DIR="$2"; shift 2 ;;
            -v) VERSION="$2"; shift 2 ;;
            -h) usage; exit 0 ;;
            *)  echo "Unknown option: $1"; usage; exit 1 ;;
        esac
    done
}

detect_os() {
    os="$(uname -s)"
    case "$os" in
        Linux)  echo "linux" ;;
        Darwin) echo "darwin" ;;
        *)      echo "Unsupported OS: $os" >&2; exit 1 ;;
    esac
}

detect_arch() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             echo "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac
}

get_latest_version() {
    # Use GitHub API to get the latest release tag
    url="https://api.github.com/repos/${REPO}/releases/latest"
    if command -v curl >/dev/null 2>&1; then
        version="$(curl -sSfL "$url" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    elif command -v wget >/dev/null 2>&1; then
        version="$(wget -qO- "$url" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    else
        echo "Error: curl or wget is required" >&2
        exit 1
    fi

    if [ -z "$version" ]; then
        echo "Error: could not determine latest version. Check https://github.com/${REPO}/releases" >&2
        exit 1
    fi
    echo "$version"
}

download() {
    url="$1"
    dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -sSfL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        echo "Error: curl or wget is required" >&2
        exit 1
    fi
}

verify_checksums() {
    checksums_file="$1"
    tarball_path="$2"
    tarball_name="$(basename "$tarball_path")"

    expected="$(grep "$tarball_name" "$checksums_file" | awk '{print $1}')"
    if [ -z "$expected" ]; then
        echo "Warning: no checksum found for $tarball_name, skipping verification" >&2
        return 0
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$tarball_path" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$tarball_path" | awk '{print $1}')"
    else
        echo "Warning: sha256sum/shasum not found, skipping checksum verification" >&2
        return 0
    fi

    if [ "$expected" != "$actual" ]; then
        echo "Error: checksum mismatch for $tarball_name" >&2
        echo "  expected: $expected" >&2
        echo "  actual:   $actual" >&2
        exit 1
    fi
}

main() {
    parse_args "$@"

    os="$(detect_os)"
    arch="$(detect_arch)"

    if [ -z "$VERSION" ]; then
        VERSION="$(get_latest_version)"
    fi

    echo "Installing ${BINARY} ${VERSION} (${os}/${arch})..."

    # Build download URL
    # Expected asset name: platform-cli_<version>_<os>_<arch>.tar.gz
    version_stripped="$(echo "$VERSION" | sed 's/^v//')"
    tarball="platform-cli_${version_stripped}_${os}_${arch}.tar.gz"
    checksums="platform-cli_${version_stripped}_checksums.txt"
    download_url="${GITHUB}/${REPO}/releases/download/${VERSION}/${tarball}"
    checksums_url="${GITHUB}/${REPO}/releases/download/${VERSION}/${checksums}"

    # Create temp directory
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT

    # Download tarball
    echo "Downloading ${download_url}..."
    download "$download_url" "${tmp}/${tarball}"

    # Download and verify checksums if available
    if download "$checksums_url" "${tmp}/${checksums}" 2>/dev/null; then
        echo "Verifying checksums..."
        verify_checksums "${tmp}/${checksums}" "${tmp}/${tarball}"
    else
        echo "Warning: checksums file not found, skipping verification" >&2
    fi

    # Extract
    tar -xzf "${tmp}/${tarball}" -C "$tmp"

    # Find the binary (may be at top level or in a subdirectory)
    binary_path=""
    if [ -f "${tmp}/${BINARY}" ]; then
        binary_path="${tmp}/${BINARY}"
    else
        binary_path="$(find "$tmp" -name "$BINARY" -type f | head -1)"
    fi

    if [ -z "$binary_path" ]; then
        echo "Error: '${BINARY}' binary not found in release archive" >&2
        exit 1
    fi

    chmod +x "$binary_path"

    # Install - try INSTALL_DIR, fall back to ~/.local/bin
    if [ -w "$INSTALL_DIR" ] || { mkdir -p "$INSTALL_DIR" 2>/dev/null && [ -w "$INSTALL_DIR" ]; }; then
        mv "$binary_path" "${INSTALL_DIR}/${BINARY}"
    elif [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
        # Try with sudo for the default path
        if command -v sudo >/dev/null 2>&1; then
            echo "Permission denied for ${INSTALL_DIR}, using sudo..."
            sudo mv "$binary_path" "${INSTALL_DIR}/${BINARY}"
        else
            # Fall back to ~/.local/bin
            INSTALL_DIR="${HOME}/.local/bin"
            mkdir -p "$INSTALL_DIR"
            mv "$binary_path" "${INSTALL_DIR}/${BINARY}"
            echo ""
            echo "Installed to ${INSTALL_DIR}/${BINARY}"
            echo "Make sure ${INSTALL_DIR} is in your PATH:"
            echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
            echo ""
            echo "${BINARY} ${VERSION} installed successfully."
            return
        fi
    else
        echo "Error: cannot write to ${INSTALL_DIR} and sudo is not available" >&2
        echo "Try: $0 -b ~/.local/bin" >&2
        exit 1
    fi

    echo ""
    echo "${BINARY} ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"

    # Quick sanity check
    if command -v "$BINARY" >/dev/null 2>&1; then
        echo "Run '${BINARY} --help' to get started."
    else
        echo ""
        echo "Note: ${INSTALL_DIR} may not be in your PATH."
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    fi
}

main "$@"
