#!/usr/bin/env bash
set -euo pipefail

REPO="jeircul/pim"
BINARY="pim"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${1:-latest}"

uname_s=$(uname -s)
case "$uname_s" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  CYGWIN*|MINGW*|MSYS*)
    echo "This installer is intended for macOS or Linux. Use install.ps1 on Windows." >&2
    exit 1
    ;;
  *)
    echo "Unsupported OS: $uname_s" >&2
    exit 1
    ;;
 esac

uname_m=$(uname -m)
case "$uname_m" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $uname_m" >&2
    exit 1
    ;;
 esac

ext="tar.gz"
asset="${BINARY}_${os}_${arch}.${ext}"
base_url="https://github.com/${REPO}/releases"

if [[ "$VERSION" == "latest" ]]; then
  download_url="${base_url}/latest/download/${asset}"
else
  [[ "$VERSION" == v* ]] || VERSION="v${VERSION}"
  download_url="${base_url}/download/${VERSION}/${asset}"
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

mkdir -p "$INSTALL_DIR"

curl -sSLf "${download_url}" -o "${workdir}/${asset}"
tar -xzf "${workdir}/${asset}" -C "$workdir"
install -m 755 "${workdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "Installed ${BINARY} to ${INSTALL_DIR}"
echo "Make sure ${INSTALL_DIR} is on your PATH."
echo "Sign in with 'az login' or 'Connect-AzAccount' before using pim. Set PIM_ALLOW_DEVICE_LOGIN=true if you need interactive fallback."
