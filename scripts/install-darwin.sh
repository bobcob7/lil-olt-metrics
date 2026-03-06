#!/usr/bin/env bash
set -euo pipefail

REPO="bobcob7/lil-olt-metrics"
BINARY_NAME="lil-olt-metrics"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/Library/Application Support/lil-olt-metrics"
CONFIG_DIR="$DATA_DIR"
PLIST_LABEL="com.bobcob7.lil-olt-metrics"
PLIST_PATH="/Library/LaunchDaemons/${PLIST_LABEL}.plist"

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Install or update lil-olt-metrics as a launchd daemon on macOS.

Options:
  --binary PATH   Use a local binary instead of downloading from GitHub
  --help          Show this help message

Examples:
  # Install latest release
  sudo $0

  # Install from a local binary
  sudo $0 --binary ./lil-olt-metrics-darwin-arm64
EOF
    exit 0
}

log() {
    echo "==> $*"
}

die() {
    echo "ERROR: $*" >&2
    exit 1
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64)  echo "amd64" ;;
        arm64)   echo "arm64" ;;
        *)       die "Unsupported architecture: $arch" ;;
    esac
}

download_latest() {
    local arch="$1"
    local asset="${BINARY_NAME}-darwin-${arch}"
    local url
    url="https://github.com/${REPO}/releases/latest/download/${asset}"
    log "Downloading ${asset} from GitHub..."
    curl -fSL -o "${INSTALL_DIR}/${BINARY_NAME}" "$url"
}

# --- Parse arguments ---
LOCAL_BINARY=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --binary)
            LOCAL_BINARY="$2"
            shift 2
            ;;
        --help)
            usage
            ;;
        *)
            die "Unknown option: $1"
            ;;
    esac
done

# --- Preflight ---
if [[ "$(uname -s)" != "Darwin" ]]; then
    die "This script is for macOS only. Use install-linux.sh for Linux."
fi
if [[ $EUID -ne 0 ]]; then
    die "This script must be run as root (use sudo)."
fi

# --- Unload existing daemon if loaded ---
if launchctl list "$PLIST_LABEL" &>/dev/null; then
    log "Unloading existing daemon..."
    launchctl bootout system "$PLIST_PATH" 2>/dev/null || true
fi

# --- Install binary ---
if [[ -n "$LOCAL_BINARY" ]]; then
    [[ -f "$LOCAL_BINARY" ]] || die "Binary not found: $LOCAL_BINARY"
    log "Installing binary from $LOCAL_BINARY..."
    cp "$LOCAL_BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
else
    ARCH="$(detect_arch)"
    download_latest "$ARCH"
fi
chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"
log "Binary installed to ${INSTALL_DIR}/${BINARY_NAME}"

# --- Create directories ---
log "Creating directories..."
mkdir -p "$DATA_DIR"

# --- Write default config (only if not present) ---
if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
    log "Writing default config to ${CONFIG_DIR}/config.yaml..."
    cat > "${CONFIG_DIR}/config.yaml" <<'YAML'
# lil-olt-metrics configuration
# See https://github.com/bobcob7/lil-olt-metrics/blob/main/docs/config-reference.md
storage:
  fs:
    path: /Library/Application Support/lil-olt-metrics
YAML
else
    log "Config already exists at ${CONFIG_DIR}/config.yaml, skipping."
fi

# --- Install launchd plist ---
log "Installing launchd plist..."
cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${PLIST_LABEL}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BINARY_NAME}</string>
        <string>-config</string>
        <string>${CONFIG_DIR}/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Library/Logs/lil-olt-metrics.log</string>
    <key>StandardErrorPath</key>
    <string>/Library/Logs/lil-olt-metrics.log</string>
    <key>WorkingDirectory</key>
    <string>${DATA_DIR}</string>
</dict>
</plist>
EOF
chmod 644 "$PLIST_PATH"

# --- Load daemon ---
log "Loading daemon..."
launchctl bootstrap system "$PLIST_PATH"
log "Daemon loaded. Check status with: sudo launchctl list ${PLIST_LABEL}"
log "Logs: /Library/Logs/lil-olt-metrics.log"
log "Installation complete."
