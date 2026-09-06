#!/bin/sh
set -eu

REPO="algebananazzzzz/Corum"
INSTALL_DIR="${CORUM_INSTALL_DIR:-$HOME/.local/bin}"
API_BASE="${_CORUM_TEST_API_BASE:-https://api.github.com}"
RELEASE_BASE="${_CORUM_TEST_RELEASE_BASE:-https://github.com/$REPO/releases/download}"

os="$(uname -s)"
case "$os" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

tag="$(curl -fsSL "$API_BASE/repos/$REPO/releases/latest" |
  grep -m1 '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
if [ -z "$tag" ]; then
  echo "could not determine the latest release of $REPO" >&2
  exit 1
fi
version="${tag#v}"

archive="corum_${version}_${os}_${arch}.tar.gz"
base="$RELEASE_BASE/$tag"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/$archive" "$base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

grep " ${archive}\$" "$tmp/checksums.txt" >"$tmp/sum" || {
  echo "no checksum entry for $archive" >&2
  exit 1
}
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$tmp" && sha256sum -c sum >/dev/null)
else
  (cd "$tmp" && shasum -a 256 -c sum >/dev/null)
fi

tar -xzf "$tmp/$archive" -C "$tmp" corum
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/corum" "$INSTALL_DIR/corum"

echo "installed corum $tag to $INSTALL_DIR/corum"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "note: $INSTALL_DIR is not on your PATH" >&2 ;;
esac
