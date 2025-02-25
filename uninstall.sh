#!/bin/bash

# Check if the script is being run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

echo 'Stopping and unloading the service with launchd'
launchctl bootout system /Library/LaunchDaemons/com.github.alankk.everythingxd.plist
if [ $? -ne 0 ]; then
    echo "Failed to unload the service. It might not be loaded."
fi

echo 'Removing launchd service plist file'
rm -f /Library/LaunchDaemons/com.github.alankk.everythingxd.plist
if [ $? -ne 0 ]; then
    echo "Failed to remove the plist file."
fi

echo 'Removing executables'
rm -f /usr/local/bin/everythingxd /usr/local/bin/ev
if [ $? -ne 0 ]; then
    echo "Failed to remove the executable."
fi

echo 'Removing EverythingX.app'
rm -rf /Applications/EverythingX.app
if [ $? -ne 0 ]; then
    echo "Failed to remove the app."
fi

echo 'Removing data directory'
rm -rf /var/lib/everythingx
if [ $? -ne 0 ]; then
    echo "Failed to remove the data directory."
fi

# You may also remove /var/log/everythingx.log

echo 'Uninstallation complete.'
