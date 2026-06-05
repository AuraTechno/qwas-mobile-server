#!/bin/bash
set -e

DB_PASS="QwasApp2026_SecurePass"

echo "=== Create qwas_app user and database ==="
sudo -u postgres psql <<EOF
DO \$\$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'qwas_app') THEN
      CREATE USER qwas_app WITH PASSWORD '${DB_PASS}';
   END IF;
END
\$\$;

SELECT 'CREATE DATABASE qwas_app OWNER qwas_app'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'qwas_app')\gexec

GRANT ALL PRIVILEGES ON DATABASE qwas_app TO qwas_app;
ALTER USER qwas_app CREATEDB;
EOF

echo "=== Verify ==="
sudo -u postgres psql -c "\l" | grep qwas_app
sudo -u postgres psql -c "\du" | grep qwas_app

echo "=== Test connection ==="
PGPASSWORD="${DB_PASS}" psql -h localhost -U qwas_app -d qwas_app -c "SELECT version();" 2>&1 | head -3

echo "=== Done ==="
