#!/usr/bin/env sh
set -eu

REPO_ARCHIVE_URL="${GAMEPANEL_ARCHIVE_URL:-https://github.com/smartcat999/game-panel-lite/archive/refs/heads/main.tar.gz}"
INSTALL_DIR="${GAMEPANEL_INSTALL_DIR:-$HOME/gamepanel-lite}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required. Install curl first, then run this command again."
  exit 1
fi

if ! command -v tar >/dev/null 2>&1; then
  echo "tar is required. Install tar first, then run this command again."
  exit 1
fi

TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

echo "Downloading GamePanel Lite..."
curl -fsSL "$REPO_ARCHIVE_URL" -o "$TMP_DIR/gamepanel-lite.tar.gz"

mkdir -p "$TMP_DIR/source" "$INSTALL_DIR"
tar -xzf "$TMP_DIR/gamepanel-lite.tar.gz" -C "$TMP_DIR/source" --strip-components 1

echo "Installing to $INSTALL_DIR..."
cp -R "$TMP_DIR/source/." "$INSTALL_DIR/"
chmod +x "$INSTALL_DIR/scripts/install.sh" "$INSTALL_DIR/scripts/setup-https.sh" "$INSTALL_DIR/scripts/renew-https.sh" 2>/dev/null || true

sh "$INSTALL_DIR/scripts/install.sh"

echo
echo "Install directory: $INSTALL_DIR"
