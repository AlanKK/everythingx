#!/bin/bash

set -x

# Check if the script is being run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

echo 'Installing executables'
cp -f findfilesd /usr/local/bin
chmod +x /usr/local/bin/findfilesd
cp -f everythingx /usr/local/bin
cp -f ev /usr/local/bin
chmod +x /usr/local/bin/everythingx /usr/local/bin/ev

echo 'Creating data directory /var/lib/findfiles'
mkdir /var/lib/findfiles

echo 'Installing launchd service'
cp -f /Users/alan/Documents/git/findfiles/cmd/service/com.example.findfiles.plist /Library/LaunchDaemons/com.example.findfiles.plist
chmod 644 /Library/LaunchDaemons/com.example.findfiles.plist
chown root:wheel /Library/LaunchDaemons/com.example.findfiles.plist

echo 'Registering and starting service with launchd'
launchctl bootout   system /Library/LaunchDaemons/com.example.findfiles.plist > /dev/null 2>&1
launchctl bootstrap system /Library/LaunchDaemons/com.example.findfiles.plist
launchctl start com.example.findfiles

echo 'Installation complete. See /var/log/findfiles.log for logs'

# Unload service
#launchctl bootout system /Library/LaunchDaemons/com.example.findfiles.plist
