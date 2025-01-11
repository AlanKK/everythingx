#!/bin/bash

# Check if the script is being run as root
if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root or with sudo" >&2
    exit 1
fi

echo 'Stopping and unloading the service with launchd'
launchctl bootout system /Library/LaunchDaemons/com.example.filefind.plist
if [ $? -ne 0 ]; then
    echo "Failed to unload the service. It might not be loaded."
fi

echo 'Removing launchd service plist file'
rm -f /Library/LaunchDaemons/com.example.filefind.plist
if [ $? -ne 0 ]; then
    echo "Failed to remove the plist file."
fi

echo 'Removing executable'
rm -f /usr/local/bin/filefind
if [ $? -ne 0 ]; then
    echo "Failed to remove the executable."
fi

echo 'Removing data directory'
rm -rf /var/lib/filefind
if [ $? -ne 0 ]; then
    echo "Failed to remove the data directory."
fi

echo 'Uninstallation complete.'