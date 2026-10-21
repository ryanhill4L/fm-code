#!/usr/bin/env bash
# Installs fm-code from GitHub Releases.
#   curl -fsSL https://raw.githubusercontent.com/ryanhill4L/fm-code/main/install.sh | bash
# Env: VERSION (default: latest), INSTALL_DIR (default: ~/.local/bin)
set -euo pipefail

REPO="ryanhill4L/fm-code"
ASSET="fm-code_darwin_arm64.tar.gz"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BASE_URL="${FM_CODE_BASE_URL:-}"

die() { echo "error: $*" >&2; exit 1; }

[ "$(uname -s)" = "Darwin" ] || die "fm-code needs macOS (it runs on Apple's on-device model)."
[ "$(uname -m)" = "arm64" ] || die "fm-code needs an Apple silicon Mac."

if [ -z "$BASE_URL" ]; then
  if [ -n "${VERSION:-}" ]; then
    BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
  else
    BASE_URL="https://github.com/$REPO/releases/latest/download"
  fi
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading fm-code..."
curl -fsSL "$BASE_URL/$ASSET" -o "$tmp/$ASSET" || die "download failed: $BASE_URL/$ASSET"
curl -fsSL "$BASE_URL/checksums.txt" -o "$tmp/checksums.txt" || die "could not download checksums.txt"

want="$(awk -v a="$ASSET" '$2 == a {print $1}' "$tmp/checksums.txt")"
got="$(shasum -a 256 "$tmp/$ASSET" | awk '{print $1}')"
[ -n "$want" ] && [ "$want" = "$got" ] || die "checksum mismatch, refusing to install."

tar -xzf "$tmp/$ASSET" -C "$tmp" fm-code
mkdir -p "$INSTALL_DIR"
install -m 755 "$tmp/fm-code" "$INSTALL_DIR/fm-code"
echo "Installed $("$INSTALL_DIR/fm-code" -version) to $INSTALL_DIR/fm-code"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add it to your PATH:  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc" ;;
esac

if ! command -v fm >/dev/null 2>&1; then
  echo "Note: the \`fm\` command wasn't found. You need macOS with Apple Intelligence enabled."
elif ! fm license --status 2>&1 | grep -q Agreed; then
  echo "Next: run \`fm license\`, read the terms and agree. fm-code won't run until you do."
fi
echo "Then run: fm-code"
