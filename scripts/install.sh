#!/usr/bin/env bash
set -euo pipefail

REPO="bobcob7/lil-olt-metrics"
BINARY_NAME="lil-olt-metrics"
INSTALL_DIR="/usr/local/bin"
VERSION="${LOM_VERSION:-latest}"

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Download and install lil-olt-metrics from GitHub releases.
Detects OS and architecture automatically.

Options:
  --version VERSION   Install a specific version (default: latest)
  --no-service        Skip system service setup
  --help              Show this help message

Environment:
  LOM_VERSION         Same as --version

Examples:
  curl -fsSL https://github.com/${REPO}/releases/latest/download/install.sh | sudo bash
  curl -fsSL https://github.com/${REPO}/releases/latest/download/install.sh | sudo bash -s -- --version v0.2.0
  sudo ./install.sh --no-service
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

setup_linux() {
    local config_dir="/etc/lil-olt-metrics"
    local data_dir="/var/lib/lil-olt-metrics"
    local service_user="lom"
    local service_group="lom"
    local systemd_unit="/etc/systemd/system/lil-olt-metrics.service"

    if ! getent group "$service_group" &>/dev/null; then
        log "Creating group ${service_group}..."
        groupadd --system "$service_group"
    fi
    if ! id "$service_user" &>/dev/null; then
        log "Creating user ${service_user}..."
        useradd --system --no-create-home --shell /usr/sbin/nologin \
            --gid "$service_group" "$service_user"
    fi
    mkdir -p "$config_dir" "$data_dir"
    chown "${service_user}:${service_group}" "$data_dir"
    if [[ ! -f "${config_dir}/config.yaml" ]]; then
        log "Writing default config to ${config_dir}/config.yaml..."
        cat > "${config_dir}/config.yaml" <<'YAML'
# lil-olt-metrics configuration
# See https://github.com/bobcob7/lil-olt-metrics/blob/main/docs/config-reference.md
storage:
  fs:
    path: /var/lib/lil-olt-metrics
YAML
    else
        log "Config already exists at ${config_dir}/config.yaml, skipping."
    fi
    log "Installing systemd unit..."
    cat > "$systemd_unit" <<UNIT
[Unit]
Description=lil-olt-metrics - lightweight OTLP metrics server
Documentation=https://github.com/${REPO}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${service_user}
Group=${service_group}
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -config ${config_dir}/config.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${data_dir}
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT
    systemctl daemon-reload
    systemctl enable "${BINARY_NAME}"
    systemctl start "${BINARY_NAME}"
    log "Service started. Check status with: systemctl status ${BINARY_NAME}"
}

setup_darwin() {
    local data_dir="/Library/Application Support/lil-olt-metrics"
    local config_dir="$data_dir"
    local plist_label="com.bobcob7.lil-olt-metrics"
    local plist_path="/Library/LaunchDaemons/${plist_label}.plist"

    mkdir -p "$data_dir"
    if [[ ! -f "${config_dir}/config.yaml" ]]; then
        log "Writing default config to ${config_dir}/config.yaml..."
        cat > "${config_dir}/config.yaml" <<'YAML'
# lil-olt-metrics configuration
# See https://github.com/bobcob7/lil-olt-metrics/blob/main/docs/config-reference.md
storage:
  fs:
    path: /Library/Application Support/lil-olt-metrics
YAML
    else
        log "Config already exists at ${config_dir}/config.yaml, skipping."
    fi
    log "Installing launchd plist..."
    cat > "$plist_path" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${plist_label}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${INSTALL_DIR}/${BINARY_NAME}</string>
        <string>-config</string>
        <string>${config_dir}/config.yaml</string>
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
    <string>${data_dir}</string>
</dict>
</plist>
PLIST
    chmod 644 "$plist_path"
    launchctl bootstrap system "$plist_path"
    log "Daemon loaded. Check status with: sudo launchctl list ${plist_label}"
    log "Logs: /Library/Logs/lil-olt-metrics.log"
}

# --- Parse arguments ---
SETUP_SERVICE=true

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help)
            usage
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --no-service)
            SETUP_SERVICE=false
            shift
            ;;
        *)
            die "Unknown option: $1"
            ;;
    esac
done

# --- Detect platform ---
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       die "Unsupported architecture: $ARCH" ;;
esac
case "$OS" in
    linux|darwin) ;;
    *) die "Unsupported OS: $OS" ;;
esac

log "Detected platform: ${OS}/${ARCH}"

# --- Preflight ---
if [[ $EUID -ne 0 ]]; then
    die "This script must be run as root (use sudo)."
fi
if ! command -v curl &>/dev/null; then
    die "curl is required but not found."
fi

# --- Resolve version ---
if [[ "$VERSION" == "latest" ]]; then
    log "Resolving latest version..."
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
    if [[ -z "$VERSION" ]]; then
        die "Could not determine latest version. Set --version explicitly."
    fi
fi
log "Installing version: ${VERSION}"

# --- Download binary ---
ARTIFACT="lil-olt-metrics-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARTIFACT}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

log "Downloading ${DOWNLOAD_URL}..."
if ! curl -fsSL -o "${TMPDIR}/${BINARY_NAME}" "$DOWNLOAD_URL"; then
    die "Download failed. Check that version ${VERSION} exists and has a ${OS}/${ARCH} build."
fi
chmod 755 "${TMPDIR}/${BINARY_NAME}"

# --- Stop existing service ---
if [[ "$OS" == "linux" ]] && systemctl is-active --quiet "${BINARY_NAME}" 2>/dev/null; then
    log "Stopping existing ${BINARY_NAME} service..."
    systemctl stop "${BINARY_NAME}"
fi
if [[ "$OS" == "darwin" ]]; then
    local_plist_label="com.bobcob7.lil-olt-metrics"
    if launchctl list "$local_plist_label" &>/dev/null; then
        log "Unloading existing daemon..."
        launchctl bootout system "/Library/LaunchDaemons/${local_plist_label}.plist" 2>/dev/null || true
    fi
fi

# --- Install binary ---
log "Installing binary to ${INSTALL_DIR}/${BINARY_NAME}..."
mkdir -p "$INSTALL_DIR"
cp "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod 755 "${INSTALL_DIR}/${BINARY_NAME}"

if [[ "$SETUP_SERVICE" != "true" ]]; then
    log "Skipping service setup (--no-service)."
    log "Binary installed to ${INSTALL_DIR}/${BINARY_NAME}"
    log "Installation complete."
    exit 0
fi

# --- Platform-specific service setup ---
if [[ "$OS" == "linux" ]]; then
    setup_linux
elif [[ "$OS" == "darwin" ]]; then
    setup_darwin
fi

log "Installation complete."
