#!/bin/bash
set -e

echo "=== Cleaning up bad Caddy config ==="
rm -f /etc/apt/sources.list.d/caddy-stable.list
rm -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg

echo "=== Install Caddy (correct method) ==="
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian/dists/any-version/InRelease' > /tmp/caddy-inrelease
gpg --dearmor < /tmp/caddy-inrelease > /usr/share/keyrings/caddy-stable-inrelease.gpg
ARCH=$(dpkg --print-architecture)
echo "deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] https://dl.cloudsmith.io/public/caddy/stable/debian/any-version any-version main" > /etc/apt/sources.list.d/caddy-stable.list
apt-get update -qq
apt-get install -y -qq caddy
caddy version

echo "=== Configure Caddy to not auto-start (we'll start manually) ==="
systemctl stop caddy 2>/dev/null || true
systemctl disable caddy 2>/dev/null || true

echo "=== Done ==="
