#!/usr/bin/env bash
set -euo pipefail

REPO="${VPSCOOP_REPO:-Jlan45/VPSCoop}"
INSTALL_DIR="${VPSCOOP_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${VPSCOOP_DATA_DIR:-/var/lib/vpscoop}"
ADDR="${VPSCOOP_ADDR:-:8090}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

if [[ "$OS" != "linux" && "$OS" != "darwin" ]]; then
  echo "Unsupported OS: $OS" >&2
  exit 1
fi

BIN_NAME="vpscoop-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${BIN_NAME}"

echo "==> Installing VPSCoop (${OS}/${ARCH}) from ${REPO}..."
TMP_BIN="$(mktemp)"
trap 'rm -f "$TMP_BIN"' EXIT

if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_BIN"; then
  echo "Failed to download ${DOWNLOAD_URL}. Make sure a GitHub Release tag (e.g. v1.0.0) has been published." >&2
  exit 1
fi
chmod +x "$TMP_BIN"

if [[ -w "$INSTALL_DIR" ]]; then
  mv "$TMP_BIN" "${INSTALL_DIR}/vpscoop"
else
  sudo mv "$TMP_BIN" "${INSTALL_DIR}/vpscoop"
fi
echo "==> Installed binary to ${INSTALL_DIR}/vpscoop"

if [[ "$OS" == "linux" ]] && command -v systemctl >/dev/null 2>&1; then
  echo "==> Configuring systemd service (vpscoop.service)..."
  sudo mkdir -p "$DATA_DIR"
  sudo chmod 700 "$DATA_DIR"

  sudo tee /etc/systemd/system/vpscoop.service >/dev/null <<EOF
[Unit]
Description=VPSCoop SSH & VPS Management Panel
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/vpscoop
Environment=VPSCOOP_ADDR=${ADDR}
Environment=VPSCOOP_DATA_DIR=${DATA_DIR}
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable vpscoop.service
  sudo systemctl restart vpscoop.service
  echo "==> VPSCoop is running! Access http://localhost${ADDR}"
else
  mkdir -p "$DATA_DIR"
  echo "==> Run 'VPSCOOP_DATA_DIR=${DATA_DIR} VPSCOOP_ADDR=${ADDR} ${INSTALL_DIR}/vpscoop' to start."
fi
