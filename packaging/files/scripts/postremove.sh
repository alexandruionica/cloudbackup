#!/bin/sh
# Reload systemd after the unit file is gone. Intentionally does NOT delete
# the cloudbackup user or /var/lib/cloudbackup — backup metadata must survive
# accidental package removal. Admins can purge state manually if desired:
#     sudo userdel cloudbackup
#     sudo groupdel cloudbackup
#     sudo rm -rf /var/lib/cloudbackup
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

exit 0
