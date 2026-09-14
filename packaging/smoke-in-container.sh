#!/usr/bin/env bash
# Runs inside the distro container started by smoke-test.sh. See that file for what is checked.
set -euo pipefail

PKG="${1:?package path inside the container}"
# the sample config ships with a placeholder; this is the bcrypt hash of the password below
HASH='$2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.'
PASSWORD='HV}H/y?<9$]Z5N4N'
ADDR="http://127.0.0.1:8080"
CLIENT_OPTS=(-a "${ADDR}" -u admin -p "${PASSWORD}")

fail() { echo "SMOKE FAIL: $*" >&2; exit 1; }
step() { echo; echo "---- $*"; }

case "${PKG}" in
    *.deb) FAMILY=deb ;;
    *.rpm) FAMILY=rpm ;;
    *) fail "unknown package type: ${PKG}" ;;
esac

step "install ${PKG}"
if [ "${FAMILY}" = deb ]; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends "${PKG}" >/dev/null
else
    dnf -y -q install "${PKG}" >/dev/null
fi

step "installed files and ownership"
[ -x /usr/bin/cloudbackup ] || fail "/usr/bin/cloudbackup missing or not executable"
cloudbackup server version | grep -q '^Server version:' || fail "server version does not run"
getent passwd cloudbackup >/dev/null || fail "service user 'cloudbackup' was not created"
[ "$(stat -c '%U:%G %a' /var/lib/cloudbackup)" = "cloudbackup:cloudbackup 750" ] || fail "/var/lib/cloudbackup ownership/mode: $(stat -c '%U:%G %a' /var/lib/cloudbackup)"
[ "$(stat -c '%U:%G %a' /etc/cloudbackup/config.yaml)" = "root:cloudbackup 640" ] || fail "config ownership/mode: $(stat -c '%U:%G %a' /etc/cloudbackup/config.yaml)"
[ -f /usr/share/cloudbackup/webstatic/ui/index.html ] || fail "web UI assets missing"
[ -f /usr/share/cloudbackup/webstatic/docs/index.html ] || fail "user guide missing"
[ -f /usr/share/cloudbackup/webstatic/docs_api/swagger.json ] || fail "swagger spec missing"
[ -f /usr/lib/systemd/system/cloudbackup.service ] || fail "systemd unit missing"
if command -v systemd-analyze >/dev/null 2>&1; then
    # only the unit's own problems count: a bare container lacks the targets it depends on
    out="$(systemd-analyze verify /usr/lib/systemd/system/cloudbackup.service 2>&1 || true)"
    echo "${out}" | grep -E '^cloudbackup\.service:|cloudbackup\.service:[0-9]+:' && fail "systemd unit fails verification"
fi

step "configure and validate as the post-install message instructs"
grep -q REPLACE_WITH_BCRYPT_HASH /etc/cloudbackup/config.yaml || fail "sample config has no placeholder hash"
sed -i "s|REPLACE_WITH_BCRYPT_HASH|${HASH}|" /etc/cloudbackup/config.yaml
cloudbackup server config validate -c /etc/cloudbackup/config.yaml

step "start the daemon the way the unit does"
EXEC="$(sed -n 's/^ExecStart=//p' /usr/lib/systemd/system/cloudbackup.service)"
[ -n "${EXEC}" ] || fail "no ExecStart in the unit"
echo "ExecStart: ${EXEC}"
${EXEC} >/tmp/cloudbackup.log 2>&1 &
DAEMON=$!
for _ in $(seq 1 100); do
    if cloudbackup client server-version "${CLIENT_OPTS[@]}" >/dev/null 2>&1; then break; fi
    kill -0 "${DAEMON}" 2>/dev/null || { cat /tmp/cloudbackup.log; fail "daemon exited early"; }
    sleep 0.2
done
cloudbackup client server-version "${CLIENT_OPTS[@]}" | grep -q '^Server version:' || { cat /tmp/cloudbackup.log; fail "client cannot reach the daemon"; }
cloudbackup client backup list --json "${CLIENT_OPTS[@]}" | grep -q '"result"' || fail "backup list over the API failed"
# the sample config ships with the placeholder credentials only: a wrong password must be refused
if cloudbackup client backup list -a "${ADDR}" -u admin -p wrong >/dev/null 2>&1; then
    fail "the API accepted a wrong password"
fi
kill "${DAEMON}"
wait "${DAEMON}" || true

step "remove the package"
if [ "${FAMILY}" = deb ]; then
    apt-get remove -y -qq cloudbackup >/dev/null
else
    dnf -y -q remove cloudbackup >/dev/null
fi
[ ! -e /usr/bin/cloudbackup ] || fail "binary still present after removal"
[ ! -e /usr/lib/systemd/system/cloudbackup.service ] || fail "unit still present after removal"
# backup metadata must survive an accidental removal (see postremove.sh)
[ -d /var/lib/cloudbackup ] || fail "/var/lib/cloudbackup was deleted on package removal"
getent passwd cloudbackup >/dev/null || fail "service user was deleted on package removal"
# the edited config is kept (deb conffile) or preserved as .rpmsave (rpm config|noreplace)
[ -e /etc/cloudbackup/config.yaml ] || [ -e /etc/cloudbackup/config.yaml.rpmsave ] || fail "edited config was discarded on removal"

echo
echo "SMOKE OK: $(basename "${PKG}")"
