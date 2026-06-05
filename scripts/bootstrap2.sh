#!/bin/bash
set -e

echo "=== Start PostgreSQL ==="
systemctl enable postgresql
systemctl start postgresql
systemctl status postgresql --no-pager | head -3

echo "=== Configure UFW ==="
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp comment 'SSH'
ufw allow 80/tcp comment 'HTTP'
ufw allow 443/tcp comment 'HTTPS'
ufw allow 3478/tcp comment 'coturn TCP'
ufw allow 3478/udp comment 'coturn UDP'
ufw allow 49152:65535/udp comment 'coturn media range'
ufw --force enable
ufw status

echo "=== Create directories ==="
mkdir -p /opt/qwas-mobile-server
mkdir -p /var/www/qwas-app-releases
mkdir -p /var/log/qwas-app
chmod 755 /opt/qwas-mobile-server
chmod 755 /var/www/qwas-app-releases

echo "=== Configure coturn (basic) ==="
cat > /etc/turnserver.conf <<'EOF'
# QWAS Mobile TURN/STUN Server
listening-port=3478
tls-listening-port=5349
realm=api-qwas.academinctools.pw
use-auth-secret
static-auth-secret=QwasTurnSecret_$(openssl rand -hex 16)
external-ip=45.10.41.65
min-port=49152
max-port=65535
fingerprint
no-multicast-peers
no-cli
log-file=/var/log/qwas-app/turnserver.log
simple-log
EOF

systemctl enable coturn
systemctl start coturn
sleep 2
systemctl status coturn --no-pager | head -3

echo "=== Final verify ==="
echo "Go: $(/usr/local/go/bin/go version)"
echo "PostgreSQL: $(psql --version)"
echo "Caddy: $(caddy version)"
echo "coturn: $(/usr/sbin/turnserver -V 2>&1 | head -1)"
echo "UFW: $(ufw status | head -1)"
echo ""
echo "=== Bootstrap complete ==="
