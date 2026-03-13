#!/bin/bash
set -e

if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo 'Installing executables'
cp -f "${SCRIPT_DIR}/bin/everythingxd" /usr/local/bin/everythingxd
chmod +x /usr/local/bin/everythingxd
cp -f "${SCRIPT_DIR}/bin/ev" /usr/local/bin/ev
chmod +x /usr/local/bin/ev

echo 'Creating data directory /var/lib/everythingx'
mkdir -p /var/lib/everythingx

echo 'Installing systemd service'
cp -f "${SCRIPT_DIR}/cmd/service/everythingxd.service" /etc/systemd/system/everythingxd.service
chmod 644 /etc/systemd/system/everythingxd.service

echo 'Enabling and starting service'
systemctl daemon-reload
systemctl enable everythingxd
systemctl start everythingxd

echo 'Installation complete. See /var/log/everythingxd.log for logs'
echo 'Use "systemctl status everythingxd" to check service status'
