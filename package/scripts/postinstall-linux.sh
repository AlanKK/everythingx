#!/bin/bash
# Runs as root after the .deb/.rpm payload is laid down. Enable and start the
# daemon under systemd. Best-effort: hosts without systemd (containers, some
# minimal distros) must not fail the whole package install, so guard on
# systemctl and never let a failed start abort with a non-zero exit.
set -u

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable everythingxd || true
    systemctl start everythingxd || true
else
    echo 'systemd not detected; skipping service registration.'
    echo 'Start the daemon manually with: sudo everythingxd'
fi

exit 0
