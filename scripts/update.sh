#!/bin/bash
# QWAS Mobile Server - One-command update
# Pulls latest, rebuilds, restarts service

set -e

APP_DIR="/opt/qwas-mobile-server"
SERVICE_NAME="qwas-app"
BIN_NAME="qwas-app-server"

cd "$APP_DIR"

echo "[1/5] Pulling latest code..."
git pull origin main

echo "[2/5] Building..."
export PATH=$PATH:/usr/local/go/bin
go build -o "$BIN_NAME" ./cmd/server
chmod +x "$BIN_NAME"

echo "[3/5] Running migrations..."
# Will be handled by the server on startup, but we can do a dry check
./"$BIN_NAME" --migrate 2>/dev/null || true

echo "[4/5] Restarting service..."
if systemctl is-active --quiet "$SERVICE_NAME"; then
    systemctl restart "$SERVICE_NAME"
    echo "Service restarted"
else
    echo "Service not running, starting..."
    systemctl start "$SERVICE_NAME"
fi

echo "[5/5] Health check..."
sleep 2
if curl -fs https://api-qwas.academinctools.pw/health > /dev/null; then
    echo "OK: server is healthy"
    echo ""
    curl -s https://api-qwas.academinctools.pw/health
    echo ""
else
    echo "WARNING: health check failed"
    systemctl status "$SERVICE_NAME" --no-pager | head -10
    exit 1
fi
