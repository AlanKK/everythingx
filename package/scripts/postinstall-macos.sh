#!/bin/bash
# Runs as root after the pkg payload is laid down. Mirrors install.sh:
# create the data dir, fix plist ownership, then (re)register and start the
# launchd daemon. Must exit 0 or the installer reports failure, so the
# launchctl calls are best-effort.
set -u

PLIST=/Library/LaunchDaemons/com.github.alankk.everythingxd.plist
LABEL=com.github.alankk.everythingx

echo 'Creating data directory /var/lib/everythingx'
mkdir -p /var/lib/everythingx

echo 'Setting launchd plist ownership'
chmod 644 "$PLIST"
chown root:wheel "$PLIST"

echo 'Registering and starting service with launchd'
# Tear down any previous instance first so bootstrap does not fail on upgrades.
launchctl bootout   system "$PLIST" >/dev/null 2>&1 || true
launchctl bootstrap system "$PLIST" || true
launchctl start "$LABEL" || true

echo 'Installation complete. See /var/log/everythingxd.log for logs'
exit 0
