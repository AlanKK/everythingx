#!/bin/bash

# Check if the script is being run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

echo 'Installing executable'
cp -f /Users/alan/Documents/git/findfiles/cmd/service/filefind /usr/local/bin/filefind
chmod +x /usr/local/bin/filefind

echo 'Creating data directory /var/lib/filefind'
mkdir /var/lib/filefind

echo 'Installing launchd service'
cp -f /Users/alan/Documents/git/findfiles/cmd/service/filefind.plist /Library/LaunchAgents/filefind.plist
chmod 644 /Library/LaunchDaemons/com.example.filefind.plist
chown root:wheel /Library/LaunchDaemons/com.example.filefind.plist

echo 'Registering and starting service with launchd'
launchctl bootstrap system /Library/LaunchDaemons/com.example.filefind.plist
launchctl start com.example.filefind

echo 'Installation complete. See /var/log/filefind.log for logs'

# Unload service
#launchctl bootout system /Library/LaunchDaemons/com.example.filefind.plist