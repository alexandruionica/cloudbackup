#!/bin/sh
#
# Install smoke test for the macOS package, run on the Mac that built it (the release workflow's
# runner). Installs the .pkg, checks what it promises, loads the launchd daemon, talks to it with the
# CLI client, unloads it, runs uninstall.sh and checks what must survive. Must run as root (sudo).
#
#   sudo sh packaging/macos/smoke-test.sh dist/packages/cloudbackup_0.0.3_macos_arm64.pkg
set -eu

PKG="${1:?package file}"
HASH='$2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.'
PASSWORD='HV}H/y?<9$]Z5N4N'
ADDR="http://127.0.0.1:8080"
LABEL=eu.ionica.cloudbackup
PLIST="/Library/LaunchDaemons/${LABEL}.plist"
ETC_DIR=/usr/local/etc/cloudbackup
DATA_DIR=/usr/local/var/cloudbackup

fail() { echo "SMOKE FAIL: $*" >&2; exit 1; }
step() { echo; echo "---- $*"; }
client() { /usr/local/bin/cloudbackup client "$@" -a "${ADDR}" -u admin -p "${PASSWORD}"; }

[ "$(id -u)" -eq 0 ] || fail "must run as root"

step "install ${PKG}"
installer -pkg "${PKG}" -target /

step "installed files and ownership"
[ -x /usr/local/bin/cloudbackup ] || fail "/usr/local/bin/cloudbackup missing"
/usr/local/bin/cloudbackup server version | grep -q '^Server version:' || fail "server version does not run"
[ "$(stat -f '%Su:%Sg %Lp' "${DATA_DIR}")" = "root:wheel 750" ] || fail "${DATA_DIR} ownership/mode: $(stat -f '%Su:%Sg %Lp' "${DATA_DIR}")"
[ "$(stat -f '%Su:%Sg %Lp' "${ETC_DIR}/config.yaml")" = "root:wheel 640" ] || fail "config ownership/mode"
[ -f /usr/local/share/cloudbackup/webstatic/ui/index.html ] || fail "web UI assets missing"
[ -f "${PLIST}" ] || fail "launchd plist missing"
plutil -lint "${PLIST}" >/dev/null || fail "launchd plist does not parse"
pkgutil --pkg-info "${LABEL}" >/dev/null || fail "package receipt missing"

step "configure and validate as the post-install message instructs"
grep -q REPLACE_WITH_BCRYPT_HASH "${ETC_DIR}/config.yaml" || fail "config has no placeholder hash"
sed -i '' "s|REPLACE_WITH_BCRYPT_HASH|${HASH}|" "${ETC_DIR}/config.yaml"
/usr/local/bin/cloudbackup server config validate -c "${ETC_DIR}/config.yaml"

step "load the launchd daemon"
launchctl enable "system/${LABEL}"
launchctl bootstrap system "${PLIST}"
i=0
while ! client server-version >/dev/null 2>&1; do
    i=$((i + 1)); [ "${i}" -lt 100 ] || { launchctl print "system/${LABEL}" || true; fail "daemon did not answer within 20s"; }
    sleep 0.2
done
launchctl print "system/${LABEL}" | grep -q 'state = running' || fail "launchd does not report the daemon running"
client server-version | grep -q '^Server version:' || fail "client cannot reach the daemon"
client backup list --json | grep -q '"result"' || fail "backup list over the API failed"
if /usr/local/bin/cloudbackup client backup list -a "${ADDR}" -u admin -p wrong >/dev/null 2>&1; then
    fail "the API accepted a wrong password"
fi
launchctl bootout "system/${LABEL}"
sleep 1
if client server-version >/dev/null 2>&1; then fail "daemon still answering after bootout"; fi

step "uninstall"
sh /usr/local/share/cloudbackup/uninstall.sh 2>/dev/null || sh "$(dirname "$0")/uninstall.sh"
[ ! -e /usr/local/bin/cloudbackup ] || fail "binary still present after uninstall"
[ ! -e "${PLIST}" ] || fail "plist still present after uninstall"
[ -d "${DATA_DIR}" ] || fail "${DATA_DIR} was deleted without --purge"
[ -f "${ETC_DIR}/config.yaml" ] || fail "edited config was discarded without --purge"
if pkgutil --pkg-info "${LABEL}" >/dev/null 2>&1; then fail "package receipt still registered"; fi

echo
echo "SMOKE OK: ${PKG}"
