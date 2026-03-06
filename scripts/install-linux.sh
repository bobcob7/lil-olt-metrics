#!/usr/bin/env bash
set -euo pipefail

REPO="bobcob7/lil-olt-metrics"
BINARY_NAME="lil-olt-metrics"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/lil-olt-metrics"
DATA_DIR="/var/lib/lil-olt-metrics"
SERVICE_USER="lom"
SERVICE_GROUP="lom"
SYSTEMD_UNIT="/etc/systemd/system/lil-olt-metrics.service"

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Build and install lil-olt-metrics as a systemd service on Linux.
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
if [[ "$(uname -s)" != "Linux" ]]; then
    die "This script is for Linux only. Use install-darwin.sh for macOS."
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

# --- Stop existing service if running ---
if systemctl is-active --quiet "${BINARY_NAME}" 2>/dev/null; then
    log "Stopping existing ${BINARY_NAME} service..."
    systemctl stop "${BINARY_NAME}"
fi

# --- Install binary ---
log "Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
cp "bin/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"

# --- Create system user/group ---
if ! getent group "$SERVICE_GROUP" &>/dev/null; then
    log "Creating group ${SERVICE_GROUP}..."
    groupadd --system "$SERVICE_GROUP"
fi
if ! id "$SERVICE_USER" &>/dev/null; then
    log "Creating user ${SERVICE_USER}..."
    useradd --system --no-create-home --shell /usr/sbin/nologin \
        --gid "$SERVICE_GROUP" "$SERVICE_USER"
fi

# --- Create directories ---
log "Creating directories..."
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chown "${SERVICE_USER}:${SERVICE_GROUP}" "$DATA_DIR"

# --- Write default config (only if not present) ---
if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
    log "Writing default config to ${CONFIG_DIR}/config.yaml..."
    cat > "${CONFIG_DIR}/config.yaml" <<'YAML'
# lil-olt-metrics configuration
# See https://github.com/bobcob7/lil-olt-metrics/blob/main/docs/config-reference.md
storage:
  fs:
    path: /var/lib/lil-olt-metrics
YAML
else
    log "Config already exists at ${CONFIG_DIR}/config.yaml, skipping."
fi

# --- Install systemd unit ---
log "Installing systemd unit..."
cat > "$SYSTEMD_UNIT" <<EOF
[Unit]
Description=lil-olt-metrics - lightweight OTLP metrics server
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -config ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# --- Enable and start ---
log "Reloading systemd and enabling service..."
systemctl daemon-reload
systemctl enable "${BINARY_NAME}"
systemctl start "${BINARY_NAME}"
log "Service started. Check status with: systemctl status ${BINARY_NAME}"
log "Installation complete."
