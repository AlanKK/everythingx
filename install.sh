#!/bin/bash

set -x

# Check if the script is being run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

echo 'Installing executables'
cp -f bin/everythingxd /usr/local/bin
chmod +x /usr/local/bin/everythingxd
cp -f bin/everythingx /usr/local/bin
cp -f bin/ev /usr/local/bin
chmod +x /usr/local/bin/everythingx /usr/local/bin/ev

echo 'Creating data directory /var/lib/everythingx'
mkdir /var/lib/everythingx

echo 'Installing launchd service'
cp -f /Users/alan/Documents/git/everythingx/cmd/service/com.example.everythingxd.plist /Library/LaunchDaemons/com.example.everythingxd.plist
chmod 644 /Library/LaunchDaemons/com.example.everythingxd.plist
chown root:wheel /Library/LaunchDaemons/com.example.everythingxd.plist

echo 'Registering and starting service with launchd'
launchctl bootout   system /Library/LaunchDaemons/com.example.everythingxd.plist > /dev/null 2>&1
launchctl bootstrap system /Library/LaunchDaemons/com.example.everythingxd.plist
launchctl start com.example.everythingx

echo 'Installation complete. See /var/log/everythingx.log for logs'

# Unload service
#launchctl bootout system /Library/LaunchDaemons/com.example.everythingxd.plist
