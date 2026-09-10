#!/bin/sh
# Shipped in the payload as /usr/local/share/cloudbackup/uninstall.sh
#
# A macOS .pkg has no uninstall phase and no "remove" verb -- Installer.app only
# ever adds files. This script is therefore the macOS analogue of the deb/rpm
# preremove+postremove pair and of pkg(8)'s +PRE_DEINSTALL/+POST_DEINSTALL.
#
# Like every other platform, the config and the backup metadata are kept by
# default; backup metadata must survive an uninstall. Pass --purge to remove
# those too.
set -e

LABEL=eu.ionica.cloudbackup
PLIST="/Library/LaunchDaemons/${LABEL}.plist"
ETC_DIR=/usr/local/etc/cloudbackup
DATA_DIR=/usr/local/var/cloudbackup
LOG_FILE=/usr/local/var/log/cloudbackup.log

PURGE=no
case "${1:-}" in
    --purge) PURGE=yes ;;
    "")      ;;
    *)       echo "usage: $0 [--purge]" >&2; exit 2 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
    echo "This script must run as root: sudo $0 ${1:-}" >&2
    exit 1
fi

echo "Stopping ${LABEL} ..."
/bin/launchctl bootout "system/${LABEL}" 2>/dev/null || true
/bin/launchctl disable "system/${LABEL}" 2>/dev/null || true

echo "Removing program files ..."
rm -f "${PLIST}"
rm -f /usr/local/bin/cloudbackup
rm -rf /usr/local/share/cloudbackup
rm -rf /usr/local/share/doc/cloudbackup
rm -f "${ETC_DIR}/config.yaml.sample"

# Drop the installer receipt so a later reinstall is not treated as a downgrade.
pkgutil --forget "${LABEL}" >/dev/null 2>&1 || true

if [ "${PURGE}" = "yes" ]; then
    echo "Purging configuration, logs and backup metadata ..."
    rm -rf "${ETC_DIR}" "${DATA_DIR}" "${LOG_FILE}"
    echo "CloudBackup fully removed."
else
    # rmdir, not rm -rf: this removes the directory only if the admin kept no
    # customised config in it.
    rmdir "${ETC_DIR}" 2>/dev/null || true
    echo
    echo "CloudBackup removed. Kept, in case you reinstall:"
    [ -e "${ETC_DIR}/config.yaml" ] && echo "  ${ETC_DIR}/config.yaml"
    [ -e "${DATA_DIR}" ]            && echo "  ${DATA_DIR} (backup metadata)"
    [ -e "${LOG_FILE}" ]            && echo "  ${LOG_FILE}"
    echo
    echo "Re-run with --purge to delete those as well."
fi

exit 0
