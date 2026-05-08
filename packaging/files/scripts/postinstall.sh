#!/bin/sh
# Set ownership and permissions on data dir and config; reload systemd.
# Service is enabled but NOT started — admin must edit /etc/cloudbackup/config.yaml
# and replace the placeholder bcrypt hash before first start.
set -e

mkdir -p /var/lib/cloudbackup
chown cloudbackup:cloudbackup /var/lib/cloudbackup
chmod 0750 /var/lib/cloudbackup

if [ -f /etc/cloudbackup/config.yaml ]; then
    chown root:cloudbackup /etc/cloudbackup/config.yaml
    chmod 0640 /etc/cloudbackup/config.yaml
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable cloudbackup.service || true
fi

cat <<'EOF'

CloudBackup is installed but not started.

Next steps:
  1. Generate a password hash:
         cloudbackup misc hash-password
  2. Edit /etc/cloudbackup/config.yaml and replace REPLACE_WITH_BCRYPT_HASH
     with the hash from step 1. Add backup definitions as needed.
  3. Validate the config:
         cloudbackup server config validate -c /etc/cloudbackup/config.yaml
  4. Start the service:
         sudo systemctl start cloudbackup

EOF

exit 0
