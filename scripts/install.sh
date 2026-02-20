#!/usr/bin/env sh
set -eu

REPO="${CVPS_REPO:-Achronon/cvps}"
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
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

binary_url="${base_url}/${asset}"
checksums_url="${base_url}/checksums.txt"

install_dir="${CVPS_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -w "/usr/local/bin" ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
  fi
fi

mkdir -p "$install_dir"
tmp_binary="$(mktemp)"
tmp_checksums="$(mktemp)"

cleanup() {
  rm -f "$tmp_binary" "$tmp_checksums"
}
trap cleanup EXIT INT TERM

download() {
  src="$1"
  dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$src" -o "$dest"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$src"
    return
  fi
  echo "curl or wget is required"
  exit 1
}

sha256_file() {
  file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
    return
  fi
  echo "sha256sum or shasum is required for checksum verification" >&2
  exit 1
}

echo "Downloading ${binary_url}"
download "$binary_url" "$tmp_binary"

echo "Downloading ${checksums_url}"
download "$checksums_url" "$tmp_checksums"

expected_sha="$(awk -v file="$asset" '$2==file {print $1}' "$tmp_checksums")"
if [ -z "$expected_sha" ]; then
  echo "Could not find checksum entry for ${asset}"
  exit 1
fi

actual_sha="$(sha256_file "$tmp_binary")"
if [ "$actual_sha" != "$expected_sha" ]; then
  echo "Checksum verification failed for ${asset}"
  echo "Expected: ${expected_sha}"
  echo "Actual:   ${actual_sha}"
  exit 1
fi

chmod +x "$tmp_binary"
mv "$tmp_binary" "${install_dir}/${BIN_NAME}"

echo "Installed ${BIN_NAME} to ${install_dir}/${BIN_NAME}"
if ! command -v cvps >/dev/null 2>&1; then
  echo ""
  echo "Add ${install_dir} to your PATH:"
  echo "  export PATH=\"${install_dir}:\$PATH\""
fi
echo ""
echo "Verify:"
echo "  cvps version"
