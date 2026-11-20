#!/bin/bash
# Runs as root after the .deb/.rpm payload is laid down. dpkg passes
# "configure"; rpm passes a number (1 install, 2 upgrade). On rpm the daemon
# is started from posttrans-rpm.sh instead: rpm runs the old package's
# preremove after this script, and an older package's preremove stops and
# disables the daemon, so a start here would be undone.
#
# restart, not start: on a deb upgrade the old daemon is still running (the
# old preremove leaves it up), and start would keep the old binary.
# Best-effort: hosts without systemd (containers, some minimal distros) must
# not fail the whole package install, so guard on systemctl and never let a
# failed start abort with a non-zero exit.
set -u

case "${1:-}" in
    [0-9]*) ;;
    *)
        if command -v systemctl >/dev/null 2>&1; then
            systemctl daemon-reload || true
            systemctl enable everythingxd || true
            systemctl restart everythingxd || true
        else
            echo 'systemd not detected; skipping service registration.'
            echo 'Start the daemon manually with: sudo everythingxd'
        fi
        ;;
esac

# Refresh launcher caches so the desktop entry and its hicolor icon are picked
# up without a re-login. Best-effort: these tools are absent on minimal hosts.
if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database -q /usr/share/applications || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || true
fi

exit 0
