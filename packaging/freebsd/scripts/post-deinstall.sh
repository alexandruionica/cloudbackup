#!/bin/sh
# Installed into the package as +POST_DEINSTALL. pkg runs it after the files
# are removed, as: sh +POST_DEINSTALL <pkg-name> POST-DEINSTALL
#
# Intentionally does NOT delete the cloudbackup user or /var/db/cloudbackup --
# backup metadata must survive accidental package removal, matching
# packaging/files/scripts/postremove.sh. Admins can purge state manually:
#     sudo pw userdel cloudbackup
#     sudo pw groupdel cloudbackup
#     sudo rm -rf /var/db/cloudbackup
set -e

# Only succeeds if the admin kept no customised config.yaml in there.
rmdir /usr/local/etc/cloudbackup 2>/dev/null || true

exit 0
