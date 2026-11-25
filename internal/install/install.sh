#!/bin/sh
# GoodVibes daemon installer.
#   curl -fsSL __GOODVIBES_SERVER__/install.sh | sh -s -- YOUR_API_KEY
set -eu

SERVER="__GOODVIBES_SERVER__"
API_KEY="${1:-${GOODVIBES_API_KEY:-}}"
RELEASE_BASE="${GOODVIBES_RELEASE_BASE:-https://github.com/Heinrich-XIAO/goodvibes/releases/latest/download}"
HOME_DIR="${GOODVIBES_HOME:-$HOME/.goodvibes}"
BIN_DIR="$HOME_DIR/bin"
BIN="$BIN_DIR/goodvibes"

die() {
  echo "goodvibes install: $*" >&2
  exit 1
}

[ -n "$API_KEY" ] || die "missing API key. Run: curl -fsSL $SERVER/install.sh | sh -s -- YOUR_API_KEY"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "unsupported OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

mkdir -p "$BIN_DIR"
echo "Downloading goodvibes ($os/$arch)..."
curl -fsSL "$RELEASE_BASE/goodvibes-$os-$arch" -o "$BIN.new"
chmod +x "$BIN.new"

# Stop the daemon from an earlier install so the new binary can take over the port.
pkill -f "$BIN daemon" 2>/dev/null || true
sleep 1
mv -f "$BIN.new" "$BIN"

"$BIN" login --server "$SERVER" --api-key "$API_KEY" >/dev/null
nohup "$BIN" daemon >"$HOME_DIR/daemon.log" 2>&1 </dev/null &

sleep 2
if "$BIN" status >/dev/null 2>&1; then
  echo "GoodVibes daemon is running and tracking. Log: $HOME_DIR/daemon.log"
else
  die "daemon did not start. See $HOME_DIR/daemon.log"
fi

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "To run 'goodvibes' from any terminal, add it to your PATH: export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac
