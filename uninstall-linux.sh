#!/bin/bash

if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

echo 'Stopping and disabling service'
systemctl stop everythingxd 2>/dev/null || echo "Service was not running."
systemctl disable everythingxd 2>/dev/null || echo "Service was not enabled."

echo 'Removing systemd service file'
rm -f /etc/systemd/system/everythingxd.service
systemctl daemon-reload

echo 'Removing executables'
rm -f /usr/local/bin/everythingxd /usr/local/bin/ev

echo 'Removing data directory'
rm -rf /var/lib/everythingx

echo 'Uninstallation complete.'
