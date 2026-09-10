#!/bin/sh
# Installed into the package as "preinstall". macOS runs it before the payload
# is written, as: preinstall <pkg-path> <target-location> <target-volume>
#
# Unlike deb/rpm/pkg(8) there is no remove phase on macOS at all, so this is the
# only chance to stop a running daemon before its binary is replaced underneath
# it. Removal is handled by the shipped uninstall.sh instead.
set -e

LABEL=eu.ionica.cloudbackup

# bootout is the modern spelling; it fails harmlessly when nothing is loaded.
if /bin/launchctl print "system/${LABEL}" >/dev/null 2>&1; then
    echo "Stopping running ${LABEL} before upgrade ..."
    /bin/launchctl bootout "system/${LABEL}" 2>/dev/null || true
fi

exit 0
