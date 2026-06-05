#!/bin/bash
# Initial server bootstrap for QWAS Mobile

set -e

echo "=== [1/8] System update ==="
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get upgrade -y -qq

echo "=== [2/8] Install base packages ==="
apt-get install -y -qq \
    postgresql postgresql-contrib \
    coturn \
    ufw \
    ca-certificates \
    gnupg \
    curl \
    wget \
    git \
    build-essential \
    nginx-common \
    dnsutils \
    jq

echo "=== [3/8] Go is already installed at /usr/local/go ==="
/usr/local/go/bin/go version
printf 'export PATH=$PATH:/usr/local/go/bin:/root/go/bin\n' > /etc/profile.d/go.sh
chmod +x /etc/profile.d/go.sh

echo "=== [4/8] Install Caddy ==="
curl -1sLf "https://dl.cloudsmith.io/public/caddy/stable/gpg.key" | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null
curl -1sLf "https://dl.cloudsmith.io/public/caddy/stable/deb/debian/dists/any-version/InRelease" | tee /etc/apt/sources.list.d/caddy-stable.list > /dev/null
apt-get update -qq
apt-get install -y -qq caddy
caddy version

echo "=== [5/8] Start PostgreSQL ==="
systemctl enable postgresql
systemctl start postgresql
systemctl status postgresql --no-pager | head -3

echo "=== [6/8] Configure UFW ==="
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
ufw status numbered

echo "=== [7/8] Setup directories ==="
mkdir -p /opt/qwas-mobile-server
mkdir -p /var/www/qwas-app-releases
mkdir -p /var/log/qwas-app
chown -R postgres:postgres /var/lib/postgresql

echo "=== [8/8] Verify ==="
echo "Go: $(/usr/local/go/bin/go version)"
echo "PostgreSQL: $(psql --version)"
echo "Caddy: $(caddy version)"
echo "coturn: $(/usr/sbin/turnserver -V 2>&1 | head -1)"
echo "UFW: $(ufw status | head -1)"

echo ""
echo "=== Bootstrap complete ==="
