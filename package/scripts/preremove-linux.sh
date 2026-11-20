#!/bin/bash
# Runs before the package's files are removed, including when the package is
# being replaced by a newer version. Only stop and disable the daemon on a
# real removal: on an upgrade dpkg passes "upgrade" and rpm passes 1 (the
# number of instances left), and the new package's postinstall (deb) or
# posttrans (rpm) restarts the daemon on the new binary. Disabling it here
# on an rpm upgrade would leave it stopped, because rpm runs the old
# package's preremove after the new package's postinstall.
case "${1:-}" in
    upgrade|1) exit 0 ;;
esac

systemctl stop everythingxd 2>/dev/null || true
systemctl disable everythingxd 2>/dev/null || true
