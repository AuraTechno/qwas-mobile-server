#!/bin/bash
set -e

cat > /etc/caddy/Caddyfile <<'CADDY'
{
    email admin@academinctools.pw
}

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
caddy validate --config /etc/caddy/Caddyfile 2>&1

# Restart
systemctl restart caddy
sleep 3
systemctl status caddy --no-pager | head -8

echo "=== Caddy is up. Test: ==="
curl -sk -o /dev/null -w "HTTP %{http_code} (will be 502 until Go server starts)\n" https://api-qwas.academinctools.pw/health 2>&1
echo ""
echo "=== Cert info ==="
echo | openssl s_client -servername api-qwas.academinctools.pw -connect api-qwas.academinctools.pw:443 2>/dev/null | openssl x509 -noout -subject -issuer -dates 2>/dev/null
