#!/bin/bash

# set -x

# Check if the script is being run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo 'Installing executables'
cp -f bin/everythingxd /usr/local/bin
chmod +x /usr/local/bin/everythingxd
cp -f bin/ev /usr/local/bin
chmod +x /usr/local/bin/ev

cp -rf EverythingX.app /Applications
chmod +x /Applications/EverythingX.app

echo 'Creating data directory /var/lib/everythingx'
mkdir -p /var/lib/everythingx

echo 'Installing launchd service'
cp -f "${SCRIPT_DIR}/cmd/service/com.github.alankk.everythingxd.plist" /Library/LaunchDaemons/com.github.alankk.everythingxd.plist
chmod 644 /Library/LaunchDaemons/com.github.alankk.everythingxd.plist
chown root:wheel /Library/LaunchDaemons/com.github.alankk.everythingxd.plist

echo 'Registering and starting service with launchd'
launchctl bootout   system /Library/LaunchDaemons/com.github.alankk.everythingxd.plist > /dev/null 2>&1
launchctl bootstrap system /Library/LaunchDaemons/com.github.alankk.everythingxd.plist
launchctl start com.github.alankk.everythingx

echo 'Installation complete. See /var/log/everythingxd.log for logs'

# Unload service
#launchctl bootout system /Library/LaunchDaemons/com.github.alankk.everythingxd.plist
