#!/bin/sh
#
# Install smoke test for the FreeBSD package, run on the FreeBSD host that built it (the release
# workflow's VM, or the Vagrant box). Installs the package, checks what it promises, enables and
# starts the rc.d service, talks to the daemon with the CLI client, stops it, deletes the package and
# checks what must survive. Must run as root.
#
#   sh packaging/freebsd/smoke-test.sh dist/packages/cloudbackup-0.0.3.freebsd14.amd64.pkg
set -eu

PKG="${1:?package file}"
HASH='$2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.'
PASSWORD='HV}H/y?<9$]Z5N4N'
ADDR="http://127.0.0.1:8080"
ETC_DIR=/usr/local/etc/cloudbackup
DATA_DIR=/var/db/cloudbackup

fail() { echo "SMOKE FAIL: $*" >&2; exit 1; }
step() { echo; echo "---- $*"; }
client() { cloudbackup client "$@" -a "${ADDR}" -u admin -p "${PASSWORD}"; }

[ "$(id -u)" -eq 0 ] || fail "must run as root"

step "install ${PKG}"
pkg install -y "${PKG}"

step "installed files and ownership"
[ -x /usr/local/bin/cloudbackup ] || fail "/usr/local/bin/cloudbackup missing"
cloudbackup server version | grep -q '^Server version:' || fail "server version does not run"
pw usershow cloudbackup >/dev/null 2>&1 || fail "service user 'cloudbackup' was not created"
[ "$(stat -f '%Su:%Sg %Lp' "${DATA_DIR}")" = "cloudbackup:cloudbackup 750" ] || fail "${DATA_DIR} ownership/mode: $(stat -f '%Su:%Sg %Lp' "${DATA_DIR}")"
[ "$(stat -f '%Su:%Sg %Lp' "${ETC_DIR}/config.yaml")" = "root:cloudbackup 640" ] || fail "config ownership/mode"
[ -f "${ETC_DIR}/config.yaml.sample" ] || fail "config.yaml.sample missing"
[ -f /usr/local/share/cloudbackup/webstatic/ui/index.html ] || fail "web UI assets missing"
[ -x /usr/local/etc/rc.d/cloudbackup ] || fail "rc.d script missing"

step "configure and validate as the post-install message instructs"
grep -q REPLACE_WITH_BCRYPT_HASH "${ETC_DIR}/config.yaml" || fail "config has no placeholder hash"
sed -i '' "s|REPLACE_WITH_BCRYPT_HASH|${HASH}|" "${ETC_DIR}/config.yaml"
cloudbackup server config validate -c "${ETC_DIR}/config.yaml"

step "enable and start the service"
sysrc cloudbackup_enable=YES >/dev/null
service cloudbackup start
i=0
while ! client server-version >/dev/null 2>&1; do
    i=$((i + 1)); [ "${i}" -lt 100 ] || { service cloudbackup status || true; fail "daemon did not answer within 20s"; }
    sleep 0.2
done
service cloudbackup status | grep -q 'is running' || fail "service status does not report running"
client server-version | grep -q '^Server version:' || fail "client cannot reach the daemon"
client backup list --json | grep -q '"result"' || fail "backup list over the API failed"
if cloudbackup client backup list -a "${ADDR}" -u admin -p wrong >/dev/null 2>&1; then
    fail "the API accepted a wrong password"
fi
service cloudbackup stop
sleep 1
if client server-version >/dev/null 2>&1; then fail "daemon still answering after service stop"; fi

step "delete the package"
pkg delete -y cloudbackup
[ ! -e /usr/local/bin/cloudbackup ] || fail "binary still present after removal"
[ ! -e /usr/local/etc/rc.d/cloudbackup ] || fail "rc.d script still present after removal"
[ -d "${DATA_DIR}" ] || fail "${DATA_DIR} was deleted on package removal"
# the edited config is kept (pre-deinstall only removes a config identical to the sample)
[ -f "${ETC_DIR}/config.yaml" ] || fail "edited config was discarded on removal"

echo
echo "SMOKE OK: ${PKG}"
