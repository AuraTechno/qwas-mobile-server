#!/bin/bash
set -e

cat > /etc/caddy/Caddyfile <<'CADDY'
{
    email admin@academinctools.pw
    auto_https on
}

# API + WebSocket
api-qwas.academinctools.pw {
    reverse_proxy localhost:4000 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }

    encode gzip zstd

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
        -Server
    }
}
CADDY

# Validate
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile 2>&1
caddy fmt --overwrite /etc/caddy/Caddyfile 2>&1 || true

# Restart
systemctl restart caddy
sleep 3
systemctl status caddy --no-pager | head -10

echo "=== Caddy is up. Test: ==="
curl -sk -o /dev/null -w "HTTP %{http_code} (will be 502 until Go server starts)\n" https://api-qwas.academinctools.pw/health 2>&1
