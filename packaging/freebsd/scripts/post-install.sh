#!/bin/sh
# Installed into the package as +POST_INSTALL. pkg runs it after the files are
# unpacked, as: sh +POST_INSTALL <pkg-name> POST-INSTALL
#
# FreeBSD counterpart of packaging/files/scripts/{preinstall,postinstall}.sh.
# There is no pre-install phase here: unlike deb/rpm, nothing in the payload is
# owned by the cloudbackup user, so creating it afterwards is sufficient.
set -e

DATA_DIR=/var/db/cloudbackup
ETC_DIR=/usr/local/etc/cloudbackup

if ! pw groupshow cloudbackup >/dev/null 2>&1; then
    pw groupadd cloudbackup
fi

if ! pw usershow cloudbackup >/dev/null 2>&1; then
    pw useradd cloudbackup \
       -g cloudbackup \
       -d "${DATA_DIR}" \
       -s /usr/sbin/nologin \
       -c "CloudBackup service" \
       -w no
fi

mkdir -p "${DATA_DIR}"
chown cloudbackup:cloudbackup "${DATA_DIR}"
chmod 0750 "${DATA_DIR}"

# config.yaml.sample ships in the package; config.yaml does not, so that pkg
# never overwrites a live config on upgrade. Seed it on first install only.
if [ ! -f "${ETC_DIR}/config.yaml" ]; then
    cp -p "${ETC_DIR}/config.yaml.sample" "${ETC_DIR}/config.yaml"
fi

chown root:cloudbackup "${ETC_DIR}/config.yaml"
chmod 0640 "${ETC_DIR}/config.yaml"

cat <<'EOF'

CloudBackup is installed but not enabled.

Next steps:
  1. Generate a password hash:
         cloudbackup misc hash-password
  2. Edit /usr/local/etc/cloudbackup/config.yaml and replace
     REPLACE_WITH_BCRYPT_HASH with the hash from step 1. Add backup
     definitions as needed.
  3. Validate the config:
         cloudbackup server config validate -c /usr/local/etc/cloudbackup/config.yaml
  4. Enable and start the service:
         sysrc cloudbackup_enable=YES
         service cloudbackup start

EOF

exit 0
