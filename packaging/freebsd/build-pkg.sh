#!/bin/sh
# Build the FreeBSD .pkg. Must run ON FreeBSD: go-sqlite3 needs cgo, so there
# is no cross-compile path from the Linux release runners the way there is for
# the Windows zips. Use either the freebsd14 Vagrant VM or the vmactions VM the
# release workflow spins up.
#
# The resulting package is tagged with the building host's ABI (FreeBSD:14:amd64
# or FreeBSD:15:amd64), and pkg refuses to install a package built for a
# different major release, which is why the workflow runs this once per release.
#
# Layout (FreeBSD hier(7), not the Linux one from packaging/nfpm.yaml):
#     /usr/local/bin/cloudbackup
#     /usr/local/etc/rc.d/cloudbackup
#     /usr/local/etc/cloudbackup/config.yaml.sample
#     /usr/local/share/cloudbackup/webstatic/...
#     /usr/local/share/doc/cloudbackup/README.md
#     /var/db/cloudbackup              (created by +POST_INSTALL, never packaged)
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

if [ "$(uname -s)" != "FreeBSD" ]; then
    echo "build-pkg.sh must run on FreeBSD (got $(uname -s))" >&2
    exit 1
fi

PKG_VERSION_BASE="$(tr -d '[:space:]' < misc/version.txt)"
if [ -z "${PKG_VERSION_BASE}" ]; then
    echo "misc/version.txt is empty" >&2
    exit 1
fi
# pkg spells the package revision "<version>_<revision>", the equivalent of the
# deb "-1" / rpm ".1" release field.
PKG_REVISION="${PKG_REVISION:-1}"
PKG_VERSION="${PKG_VERSION_BASE}_${PKG_REVISION}"

ABI_MAJOR="$(uname -r | cut -d. -f1)"   # 14.5-RELEASE -> 14
PKG_ARCH="$(uname -m)"                  # amd64 on FreeBSD, not x86_64

BUILD_DIR="${ROOT}/dist/build/freebsd"
STAGE="${ROOT}/dist/freebsd/stage"
META="${ROOT}/dist/freebsd/meta"
PLIST="${ROOT}/dist/freebsd/plist"
PKG_DIR="${ROOT}/dist/packages"

rm -rf "${STAGE}" "${META}"
mkdir -p "${BUILD_DIR}" "${STAGE}" "${META}" "${PKG_DIR}"

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] generating version stamp ############"
# generate_version.sh shells out to git; in CI the tree is rsynced into a VM
# where it ends up owned by a different uid than the one running the build.
git config --global --add safe.directory "${ROOT}" 2>/dev/null || true
bash generate_version.sh

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] building binary ############"
go version
CGO_ENABLED=1 go build -v -mod=vendor -o "${BUILD_DIR}/cloudbackup" .

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] staging ############"
install -d "${STAGE}/usr/local/bin"
install -m 0755 "${BUILD_DIR}/cloudbackup" "${STAGE}/usr/local/bin/cloudbackup"

install -d "${STAGE}/usr/local/etc/rc.d"
install -m 0755 packaging/freebsd/cloudbackup.rc "${STAGE}/usr/local/etc/rc.d/cloudbackup"

# Only the .sample is packaged; +POST_INSTALL seeds config.yaml from it. That
# keeps a live config out of the plist, so pkg can never clobber it on upgrade.
install -d "${STAGE}/usr/local/etc/cloudbackup"
install -m 0644 packaging/freebsd/config.yaml.sample \
    "${STAGE}/usr/local/etc/cloudbackup/config.yaml.sample"

# Same webstatic subset the Linux packages ship (see build-in-container.sh).
WEBROOT="${STAGE}/usr/local/share/cloudbackup/webstatic"
install -d "${WEBROOT}/ui"
cp -R webstatic/docs "${WEBROOT}/docs"
cp -R webstatic/docs_api "${WEBROOT}/docs_api"
cp webstatic/ui/index.html webstatic/ui/styles.css "${WEBROOT}/ui/"
cp -R webstatic/ui/js "${WEBROOT}/ui/js"
cp -R webstatic/ui/vendor "${WEBROOT}/ui/vendor"

install -d "${STAGE}/usr/local/share/doc/cloudbackup"
install -m 0644 README.md "${STAGE}/usr/local/share/doc/cloudbackup/README.md"

# cp -R carries the working tree's modes across; normalise so the package does
# not inherit whatever umask the checkout happened to be made under.
find "${STAGE}/usr/local/share" -type d -exec chmod 0755 {} +
find "${STAGE}/usr/local/share" -type f -exec chmod 0644 {} +

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] generating plist ############"
find "${STAGE}" -type f | sed "s|^${STAGE}||" | sort > "${PLIST}"
# @dir marks directories for removal at deinstall. Restrict it to the trees we
# own: /usr/local/bin and /usr/local/etc/rc.d are shared with the rest of the
# system and must not be listed.
for owned in /usr/local/etc/cloudbackup \
             /usr/local/share/cloudbackup \
             /usr/local/share/doc/cloudbackup; do
    find "${STAGE}${owned}" -type d | sed "s|^${STAGE}||" | sort -r | \
        sed 's|^|@dir |' >> "${PLIST}"
done

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] assembling metadata ############"
sed -e "s|\${PKG_VERSION}|${PKG_VERSION}|g" \
    packaging/freebsd/manifest.template > "${META}/+MANIFEST"
install -m 0755 packaging/freebsd/scripts/post-install.sh   "${META}/+POST_INSTALL"
install -m 0755 packaging/freebsd/scripts/pre-deinstall.sh  "${META}/+PRE_DEINSTALL"
install -m 0755 packaging/freebsd/scripts/post-deinstall.sh "${META}/+POST_DEINSTALL"

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] running pkg create ############"
pkg create -m "${META}" -p "${PLIST}" -r "${STAGE}" -o "${PKG_DIR}"

# pkg names its output <name>-<version>.pkg; add the ABI and arch so the
# release page can carry one asset per FreeBSD major release.
RAW="${PKG_DIR}/cloudbackup-${PKG_VERSION}.pkg"
OUT="${PKG_DIR}/cloudbackup-${PKG_VERSION}.freebsd${ABI_MAJOR}.${PKG_ARCH}.pkg"
if [ ! -f "${RAW}" ]; then
    echo "expected pkg create to produce ${RAW}, but it did not" >&2
    ls -l "${PKG_DIR}" >&2
    exit 1
fi
mv "${RAW}" "${OUT}"

echo "############ [freebsd${ABI_MAJOR}/${PKG_ARCH}] produced ${OUT} ############"
ls -lh "${OUT}"
pkg info -F "${OUT}"
