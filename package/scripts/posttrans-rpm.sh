#!/bin/bash
# rpm only: runs once the whole transaction is done, after the old package's
# preremove. Enables the daemon and (re)starts it on the new binary, so an
# upgrade runs the new version (and any database migration) right away, even
# when an older package's preremove has just stopped and disabled it.
# Best-effort: hosts without systemd (containers, some minimal distros) must
# not fail the whole package install, so guard on systemctl and never let a
# failed start abort with a non-zero exit.
set -u

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable everythingxd || true
    systemctl restart everythingxd || true
else
    echo 'systemd not detected; skipping service registration.'
    echo 'Start the daemon manually with: sudo everythingxd'
fi

exit 0
