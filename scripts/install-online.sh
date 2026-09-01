#!/usr/bin/env sh
set -eu

GAMEPANEL_VERSION="${GAMEPANEL_VERSION:-v0.2.11}"
REPO_ARCHIVE_URL="${GAMEPANEL_ARCHIVE_URL:-https://github.com/smartcat999/game-panel-lite/archive/refs/tags/$GAMEPANEL_VERSION.tar.gz}"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/gamepanel-lite"
INSTALL_PATH_FILE="$CONFIG_DIR/install-path"

if [ -n "${1:-}" ]; then
  INSTALL_DIR="$1"
elif [ -n "${GAMEPANEL_INSTALL_DIR:-}" ]; then
  INSTALL_DIR="$GAMEPANEL_INSTALL_DIR"
else
  DEFAULT_INSTALL_DIR="$HOME/gamepanel-lite"
  if [ -r "$INSTALL_PATH_FILE" ]; then
    saved_install_dir=$(sed -n '1p' "$INSTALL_PATH_FILE")
    [ -n "$saved_install_dir" ] && DEFAULT_INSTALL_DIR="$saved_install_dir"
  fi
  if [ -r /dev/tty ] && [ -w /dev/tty ]; then
    printf "Install directory / 安装目录 [%s]: " "$DEFAULT_INSTALL_DIR" >/dev/tty
    IFS= read -r selected_install_dir </dev/tty || selected_install_dir=""
    INSTALL_DIR="${selected_install_dir:-$DEFAULT_INSTALL_DIR}"
  else
    INSTALL_DIR="$DEFAULT_INSTALL_DIR"
    echo "No interactive terminal detected; using installation directory: $INSTALL_DIR"
  fi
fi

case "$INSTALL_DIR" in
  "~") INSTALL_DIR="$HOME" ;;
  "~/"*) INSTALL_DIR="$HOME/${INSTALL_DIR#\~/}" ;;
  /*) ;;
  *) INSTALL_DIR="$(pwd)/$INSTALL_DIR" ;;
esac

if [ "$INSTALL_DIR" = "/" ]; then
  echo "Refusing to install into the filesystem root" >&2
  exit 2
fi

if [ -f "$INSTALL_DIR/.env" ]; then
  echo "Existing installation detected / 检测到已有安装: $INSTALL_DIR"
fi

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
chmod +x "$INSTALL_DIR/scripts/install.sh" "$INSTALL_DIR/scripts/setup-https.sh" "$INSTALL_DIR/scripts/renew-https.sh" "$INSTALL_DIR/scripts/install-https-renewal-timer.sh" "$INSTALL_DIR/scripts/https-renewal-runner.sh" "$INSTALL_DIR/scripts/manage.sh" 2>/dev/null || true

sh "$INSTALL_DIR/scripts/install.sh"

mkdir -p "$CONFIG_DIR"
printf '%s\n' "$INSTALL_DIR" > "$INSTALL_PATH_FILE"
chmod 600 "$INSTALL_PATH_FILE"

echo
echo "Install directory: $INSTALL_DIR"
