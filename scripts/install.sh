#!/bin/bash
# Initial server install script - run once on a fresh server
# Requires: root, apt access, /etc/qwas-app.env with secrets

set -e

APP_DIR="/opt/qwas-mobile-server"
SERVICE_NAME="qwas-app"
BIN_NAME="qwas-app-server"
REPO_URL="${QWAS_REPO_URL:-https://github.com/AuraTechno/qwas-mobile-server.git}"

echo "=== [1/6] Pre-flight ==="
if [ ! -f /etc/qwas-app.env ]; then
    echo "ERROR: /etc/qwas-app.env not found. Run scripts/setup-caddy-turn.sh first."
    exit 1
fi

if [ ! -d /var/log/qwas-app ]; then
    mkdir -p /var/log/qwas-app
fi

echo "=== [2/6] Clone repo ==="
if [ -d "$APP_DIR" ]; then
    echo "Directory exists, pulling latest..."
    cd "$APP_DIR"
    git pull origin main
else
    git clone "$REPO_URL" "$APP_DIR"
    cd "$APP_DIR"
fi

echo "=== [3/6] Build ==="
export PATH=$PATH:/usr/local/go/bin
go build -o "$BIN_NAME" ./cmd/server
chmod +x "$BIN_NAME"
echo "Built: $BIN_NAME"

echo "=== [4/6] Install systemd unit ==="
cp scripts/qwas-app.service /etc/systemd/system/qwas-app.service
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

echo "=== [5/6] Install update script ==="
cp scripts/update.sh /usr/local/bin/qwas-app-update
chmod +x /usr/local/bin/qwas-app-update
echo "Installed: /usr/local/bin/qwas-app-update"

echo "=== [6/6] Start service ==="
systemctl restart "$SERVICE_NAME"
sleep 3
systemctl status "$SERVICE_NAME" --no-pager | head -10

echo ""
echo "=== Install complete ==="
echo "Service: $SERVICE_NAME"
echo "Update command: qwas-app-update"
echo "Logs: journalctl -u $SERVICE_NAME -f"
echo "Health: https://api-qwas.academinctools.pw/health"
