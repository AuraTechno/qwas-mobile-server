#!/bin/bash
set -e

APP_DIR="/opt/qwas-mobile-server"
SERVICE_NAME="qwas-app"

echo "=== Cleaning old dir ==="
if [ -d "$APP_DIR" ]; then
    cd "$APP_DIR"
    if [ ! -d .git ]; then
        echo "Not a git repo, removing..."
        cd /
        rm -rf "$APP_DIR"
    fi
fi

echo "=== Cloning fresh ==="
if [ ! -d "$APP_DIR" ]; then
    git clone https://github.com/AuraTechno/qwas-mobile-server.git "$APP_DIR"
fi
cd "$APP_DIR"

echo "=== Build ==="
export PATH=$PATH:/usr/local/go/bin
go build -o qwas-app-server ./cmd/server
chmod +x qwas-app-server
ls -la qwas-app-server

echo "=== Install systemd ==="
cp scripts/qwas-app.service /etc/systemd/system/qwas-app.service
systemctl daemon-reload
systemctl enable qwas-app

echo "=== Install update script ==="
cp scripts/update.sh /usr/local/bin/qwas-app-update
chmod +x /usr/local/bin/qwas-app-update

echo "=== Start service ==="
systemctl restart qwas-app
sleep 3
systemctl status qwas-app --no-pager | head -15

echo "=== Health check (via Caddy HTTPS) ==="
sleep 2
curl -sk -o /tmp/health.json -w "HTTP %{http_code}\n" https://api-qwas.academinctools.pw/health
cat /tmp/health.json | head -c 200
echo ""
