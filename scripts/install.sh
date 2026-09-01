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
checksums="${BINARY}_checksums.txt"
base_url="https://github.com/${REPO}/releases"

if [[ "$VERSION" == "latest" ]]; then
  download_url="${base_url}/latest/download/${asset}"
  checksums_url="${base_url}/latest/download/${checksums}"
else
  [[ "$VERSION" == v* ]] || VERSION="v${VERSION}"
  download_url="${base_url}/download/${VERSION}/${asset}"
  checksums_url="${base_url}/download/${VERSION}/${checksums}"
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

mkdir -p "$INSTALL_DIR"

curl -sSLf "${download_url}" -o "${workdir}/${asset}"
curl -sSLf "${checksums_url}" -o "${workdir}/${checksums}"

expected=$(awk -v a="$asset" '$2 == a { print $1 }' "${workdir}/${checksums}")
if [[ -z "$expected" ]]; then
  echo "No checksum entry for ${asset} in ${checksums}" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "${workdir}/${asset}" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "${workdir}/${asset}" | awk '{ print $1 }')
else
  echo "Cannot verify download: neither sha256sum nor shasum is available." >&2
  exit 1
fi

if [[ "$actual" != "$expected" ]]; then
  echo "Checksum mismatch for ${asset}" >&2
  echo "  expected: ${expected}" >&2
  echo "  actual:   ${actual}" >&2
  exit 1
fi

tar -xzf "${workdir}/${asset}" -C "$workdir"
install -m 755 "${workdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo "Installed ${BINARY} to ${INSTALL_DIR}"
echo "Make sure ${INSTALL_DIR} is on your PATH."
echo "Sign in with 'az login' or 'Connect-AzAccount' before using pim. Set PIM_ALLOW_DEVICE_LOGIN=true if you need interactive fallback."
