#!/bin/sh
# Create the cloudbackup system user and group if missing.
# Runs before files are unpacked, so we can be sure the user exists when
# postinstall sets ownership.
set -e

if ! getent group cloudbackup >/dev/null 2>&1; then
    if command -v groupadd >/dev/null 2>&1; then
        groupadd --system cloudbackup
    else
        addgroup --system cloudbackup
    fi
fi

if ! getent passwd cloudbackup >/dev/null 2>&1; then
    if command -v useradd >/dev/null 2>&1; then
        useradd --system \
                --gid cloudbackup \
                --home-dir /var/lib/cloudbackup \
                --no-create-home \
                --shell /usr/sbin/nologin \
                --comment "CloudBackup service" \
                cloudbackup
    else
        adduser --system \
                --ingroup cloudbackup \
                --home /var/lib/cloudbackup \
                --no-create-home \
                --shell /usr/sbin/nologin \
                --gecos "CloudBackup service" \
                cloudbackup
    fi
fi

exit 0
