#!/usr/bin/env sh
set -eu

REPO="${CVPS_REPO:-Achronon/claudevps}"
VERSION="${CVPS_VERSION:-latest}"
BIN_NAME="cvps"

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin) os_slug="darwin" ;;
  Linux) os_slug="linux" ;;
  *)
    echo "Unsupported OS: $os"
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch_slug="amd64" ;;
  arm64|aarch64) arch_slug="arm64" ;;
  *)
    echo "Unsupported architecture: $arch"
    exit 1
    ;;
esac

asset="${BIN_NAME}-${os_slug}-${arch_slug}"
if [ "$VERSION" = "latest" ]; then
  download_url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  download_url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

install_dir="${CVPS_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -w "/usr/local/bin" ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
  fi
fi

mkdir -p "$install_dir"
tmp_file="$(mktemp)"

cleanup() {
  rm -f "$tmp_file"
}
trap cleanup EXIT INT TERM

echo "Downloading ${download_url}"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$download_url" -o "$tmp_file"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$tmp_file" "$download_url"
else
  echo "curl or wget is required"
  exit 1
fi

chmod +x "$tmp_file"
mv "$tmp_file" "${install_dir}/${BIN_NAME}"

echo "Installed ${BIN_NAME} to ${install_dir}/${BIN_NAME}"
if ! command -v cvps >/dev/null 2>&1; then
  echo ""
  echo "Add ${install_dir} to your PATH:"
  echo "  export PATH=\"${install_dir}:\$PATH\""
fi
echo ""
echo "Verify:"
echo "  cvps version"
