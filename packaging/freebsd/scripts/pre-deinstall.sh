#!/bin/sh
# Installed into the package as +PRE_DEINSTALL. pkg runs it before the files
# are removed, as: sh +PRE_DEINSTALL <pkg-name> DEINSTALL
#
# FreeBSD counterpart of packaging/files/scripts/preremove.sh. Unlike systemd
# there is nothing to "disable" here beyond the rc.conf knob, which is the
# admin's to own, so we only stop a running daemon.
set -e

ETC_DIR=/usr/local/etc/cloudbackup

if [ -x /usr/local/etc/rc.d/cloudbackup ]; then
    if service cloudbackup status >/dev/null 2>&1; then
        service cloudbackup stop || true
    fi
fi

# Mirror pkg's @sample semantics by hand: drop the generated config only when
# the admin never touched it. This runs while config.yaml.sample is still on
# disk, which is why the comparison cannot live in +POST_DEINSTALL.
if [ -f "${ETC_DIR}/config.yaml" ] && [ -f "${ETC_DIR}/config.yaml.sample" ]; then
    if cmp -s "${ETC_DIR}/config.yaml" "${ETC_DIR}/config.yaml.sample"; then
        rm -f "${ETC_DIR}/config.yaml"
    fi
fi

exit 0
