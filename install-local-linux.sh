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
cp -f "${SCRIPT_DIR}/bin/everythingx" /usr/local/bin/everythingx
chmod +x /usr/local/bin/everythingx

# Same desktop entry and icon layout as the .deb/.rpm (see nfpm.yaml). The
# entry's basename must match the app ID so Wayland can find its icon.
echo 'Installing desktop entry and icons'
ICONS="${SCRIPT_DIR}/assets/icons/retina/orange-black"
install -D -m 644 "${SCRIPT_DIR}/assets/com.github.alankk.everythingx.desktop" \
    /usr/share/applications/com.github.alankk.everythingx.desktop
install -D -m 644 "${ICONS}/folder-orange-black-16@2x.png"  /usr/share/icons/hicolor/32x32/apps/everythingx.png
install -D -m 644 "${ICONS}/folder-orange-black-32@2x.png"  /usr/share/icons/hicolor/64x64/apps/everythingx.png
install -D -m 644 "${ICONS}/folder-orange-black-64@2x.png"  /usr/share/icons/hicolor/128x128/apps/everythingx.png
install -D -m 644 "${ICONS}/folder-orange-black-128@2x.png" /usr/share/icons/hicolor/256x256/apps/everythingx.png
install -D -m 644 "${ICONS}/folder-orange-black-512@2x.png" /usr/share/pixmaps/everythingx.png
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi

echo 'Creating data directory /var/lib/everythingx'
mkdir -p /var/lib/everythingx

echo 'Installing systemd service'
cp -f "${SCRIPT_DIR}/cmd/service/everythingxd.service" /etc/systemd/system/everythingxd.service
chmod 644 /etc/systemd/system/everythingxd.service

echo 'Enabling and (re)starting service'
systemctl daemon-reload
systemctl enable everythingxd
# restart, not start: on a reinstall the old daemon is still running and
# start would leave it on the previous binary.
systemctl restart everythingxd

echo 'Installation complete. See /var/log/everythingxd.log for logs'
echo 'Use "systemctl status everythingxd" to check service status'
