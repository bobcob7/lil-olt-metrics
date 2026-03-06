#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="lil-olt-metrics"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/Library/Application Support/lil-olt-metrics"
CONFIG_DIR="$DATA_DIR"
PLIST_LABEL="com.bobcob7.lil-olt-metrics"
PLIST_PATH="/Library/LaunchDaemons/${PLIST_LABEL}.plist"

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Build and install lil-olt-metrics as a launchd daemon on macOS.
Must be run from the repository root.

Options:
  --help    Show this help message

Examples:
  sudo $0
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

# --- Parse arguments ---
while [[ $# -gt 0 ]]; do
    case "$1" in
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
if [[ ! -f "go.mod" ]]; then
    die "Must be run from the repository root (go.mod not found)."
fi
if ! command -v go &>/dev/null; then
    die "Go is required but not found. Install Go and retry."
fi

# --- Build binary ---
log "Building ${BINARY_NAME}..."
GOBIN="$(pwd)/bin" go build -o "bin/${BINARY_NAME}" ./cmd/server

# --- Unload existing daemon if loaded ---
if launchctl list "$PLIST_LABEL" &>/dev/null; then
    log "Unloading existing daemon..."
    launchctl bootout system "$PLIST_PATH" 2>/dev/null || true
fi

# --- Install binary ---
log "Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
cp "bin/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"

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
