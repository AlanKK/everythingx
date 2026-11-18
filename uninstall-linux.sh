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
rm -f /usr/local/bin/everythingxd /usr/local/bin/ev /usr/local/bin/everythingx

echo 'Removing desktop entry and icons'
rm -f /usr/share/applications/com.github.alankk.everythingx.desktop \
      /usr/share/icons/hicolor/{32x32,64x64,128x128,256x256}/apps/everythingx.png \
      /usr/share/pixmaps/everythingx.png
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi

echo 'Removing data directory'
rm -rf /var/lib/everythingx

echo 'Uninstallation complete.'
