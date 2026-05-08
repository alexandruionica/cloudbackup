#!/bin/sh
# Stop the service before package removal. Disable only on full uninstall,
# not on upgrade.
#
# DEB invocation: $1 is "remove", "upgrade", "deconfigure", "failed-upgrade", ...
# RPM invocation: $1 is 0 (uninstall) or 1 (upgrade).
set -e

if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet cloudbackup.service 2>/dev/null; then
        systemctl stop cloudbackup.service || true
    fi

    case "${1:-}" in
        0|remove)
            systemctl disable cloudbackup.service || true
            ;;
    esac
fi

exit 0
