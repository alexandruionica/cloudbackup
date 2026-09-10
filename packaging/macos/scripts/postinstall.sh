#!/bin/sh
# Installed into the package as "postinstall". macOS runs it after the payload
# is written, as: postinstall <pkg-path> <target-location> <target-volume>
#
# macOS counterpart of packaging/files/scripts/postinstall.sh. There is no
# system user: macOS has no useradd, creating one needs dscl plus a free-UID
# search, and the daemon runs as root on every platform anyway. The plist's
# UserName key is the supported way to drop privileges.
set -e

ETC_DIR=/usr/local/etc/cloudbackup
DATA_DIR=/usr/local/var/cloudbackup
LOG_DIR=/usr/local/var/log

mkdir -p "${DATA_DIR}" "${LOG_DIR}"
chown root:wheel "${DATA_DIR}"
chmod 0750 "${DATA_DIR}"

# Only config.yaml.sample is in the payload; seed the live config from it on
# first install so an edited config is never part of an upgrade's file set.
if [ ! -f "${ETC_DIR}/config.yaml" ]; then
    cp -p "${ETC_DIR}/config.yaml.sample" "${ETC_DIR}/config.yaml"
fi
chown root:wheel "${ETC_DIR}/config.yaml"
chmod 0640 "${ETC_DIR}/config.yaml"

cat <<'EOF'

CloudBackup is installed but not started.

Next steps:
  1. Generate a password hash:
         cloudbackup misc hash-password
  2. Edit /usr/local/etc/cloudbackup/config.yaml and replace
     REPLACE_WITH_BCRYPT_HASH with the hash from step 1. Add backup
     definitions as needed.
  3. Validate the config:
         cloudbackup server config validate -c /usr/local/etc/cloudbackup/config.yaml
  4. Enable and start the service:
         sudo launchctl enable system/eu.ionica.cloudbackup
         sudo launchctl bootstrap system /Library/LaunchDaemons/eu.ionica.cloudbackup.plist

Logs go to /usr/local/var/log/cloudbackup.log
To uninstall:  sudo /usr/local/share/cloudbackup/uninstall.sh

EOF

exit 0
