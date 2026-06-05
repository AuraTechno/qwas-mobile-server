#!/bin/bash
set -e

TURN_SECRET=$(openssl rand -hex 32)
echo "Generated TURN secret, saving to /etc/qwas-app.env"

cat > /etc/turnserver.conf <<EOF
# QWAS Mobile TURN/STUN Server
listening-port=3478
tls-listening-port=5349
realm=api-qwas.academinctools.pw
use-auth-secret
static-auth-secret=${TURN_SECRET}
external-ip=45.10.41.65
min-port=49152
max-port=65535
fingerprint
no-multicast-peers
no-cli
log-file=/var/log/qwas-app/turnserver.log
simple-log
EOF

cat > /etc/qwas-app.env <<EOF
# QWAS Mobile - secrets (DO NOT COMMIT)
QWAS_DB_HOST=localhost
QWAS_DB_PORT=5432
QWAS_DB_NAME=qwas_app
QWAS_DB_USER=qwas_app
QWAS_DB_PASS=QwasApp2026_SecurePass
QWAS_JWT_SECRET=$(openssl rand -hex 32)
QWAS_TURN_SECRET=${TURN_SECRET}
QWAS_TURN_REALM=api-qwas.academinctools.pw
QWAS_TURN_HOST=45.10.41.65
QWAS_TURN_PORT=3478
QWAS_PUBLIC_URL=https://api-qwas.academinctools.pw
QWAS_PORT=4000
EOF

chmod 600 /etc/qwas-app.env
chmod 644 /etc/turnserver.conf

systemctl restart coturn
sleep 2
systemctl status coturn --no-pager | head -3

echo "=== Caddyfile for api-qwas.academinctools.pw ==="
mkdir -p /etc/caddy
cat > /etc/caddy/Caddyfile <<'CADDY'
{
    email admin@academinctools.pw
}

api-qwas.academinctools.pw {
    reverse_proxy localhost:4000
    encode gzip zstd
    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
        X-Frame-Options "DENY"
    }
    @media path /media/*
    handle_path /media/* {
        root /var/www/qwas-app-releases
    }
    @app path /app/*
    handle_path /app/* {
        root /var/www/qwas-app-releases
    }
}

# Coturn TLS - separate subdomain (optional, can use same cert)
# turn.academinctools.pw {
#     reverse_proxy localhost:5349
# }
CADDY

# Validate Caddy config
caddy validate --config /etc/caddy/Caddyfile 2>&1 | head -5

# Restart Caddy
systemctl enable caddy
systemctl restart caddy
sleep 2
systemctl status caddy --no-pager | head -5

echo "=== Test HTTPS ==="
curl -sk -o /dev/null -w "HTTP %{http_code}\n" https://api-qwas.academinctools.pw/health 2>&1

echo "=== Done ==="
echo "TURN secret saved to /etc/qwas-app.env"
echo "JWT secret saved to /etc/qwas-app.env"
