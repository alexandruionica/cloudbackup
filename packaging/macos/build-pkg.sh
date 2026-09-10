#!/bin/sh
# Build the macOS .pkg installer. Must run ON macOS: go-sqlite3 needs cgo, and
# pkgbuild/productbuild are Xcode command line tools.
#
# Unlike FreeBSD, GitHub hosts native macOS runners, so CI builds each
# architecture on its own runner rather than inside a VM. Building the other
# architecture locally still works -- Xcode ships both SDKs -- via PKG_ARCH.
#
# Layout (macOS has no single convention for a daemon; this follows the Unix
# shape the Linux and FreeBSD packages use, which is also what Homebrew's
# pre-Apple-Silicon prefix looked like):
#     /usr/local/bin/cloudbackup
#     /usr/local/etc/cloudbackup/config.yaml.sample
#     /usr/local/share/cloudbackup/webstatic/...
#     /usr/local/share/cloudbackup/uninstall.sh
#     /usr/local/share/doc/cloudbackup/README.md
#     /Library/LaunchDaemons/eu.ionica.cloudbackup.plist
#     /usr/local/var/cloudbackup            (created by postinstall, not packaged)
#
# Signing is optional and off by default. Set MACOS_SIGN_IDENTITY to sign, and
# additionally MACOS_NOTARY_KEY_PATH / _KEY_ID / _KEY_ISSUER to notarize and
# staple. Without those the package still installs, but Gatekeeper blocks
# double-click and the user must right-click -> Open or strip the quarantine
# attribute.
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

if [ "$(uname -s)" != "Darwin" ]; then
    echo "build-pkg.sh must run on macOS (got $(uname -s))" >&2
    exit 1
fi

PKG_VERSION="$(tr -d '[:space:]' < misc/version.txt)"
if [ -z "${PKG_VERSION}" ]; then
    echo "misc/version.txt is empty" >&2
    exit 1
fi

case "$(uname -m)" in
    arm64)  HOST_ARCH=arm64 ;;
    x86_64) HOST_ARCH=amd64 ;;
    *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac
PKG_ARCH="${PKG_ARCH:-${HOST_ARCH}}"

IDENTIFIER=eu.ionica.cloudbackup
# Build against an older SDK floor than the runner's own OS so the binary is
# not accidentally pinned to the newest macOS. Override if you need a higher one.
export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-13.0}"

BUILD_DIR="${ROOT}/dist/build/macos"
STAGE="${ROOT}/dist/macos/stage"
SCRIPTS="${ROOT}/dist/macos/scripts"
PKG_DIR="${ROOT}/dist/packages"

rm -rf "${STAGE}" "${SCRIPTS}"
mkdir -p "${BUILD_DIR}" "${STAGE}" "${SCRIPTS}" "${PKG_DIR}"

echo "############ [macos/${PKG_ARCH}] generating version stamp ############"
git config --global --add safe.directory "${ROOT}" 2>/dev/null || true
bash generate_version.sh

echo "############ [macos/${PKG_ARCH}] building binary ############"
export CGO_ENABLED=1
export GOARCH="${PKG_ARCH}"
if [ "${PKG_ARCH}" != "${HOST_ARCH}" ]; then
    # Both slices' SDKs ship with Xcode, so cgo can cross-build between them as
    # long as clang is told which one to target.
    case "${PKG_ARCH}" in
        amd64) clang_arch=x86_64 ;;
        arm64) clang_arch=arm64 ;;
    esac
    echo "cross-building for ${PKG_ARCH} on ${HOST_ARCH} (clang -arch ${clang_arch})"
    export CC="clang -arch ${clang_arch}"
    export CXX="clang++ -arch ${clang_arch}"
fi
go version
go build -v -mod=vendor -o "${BUILD_DIR}/cloudbackup" .
file "${BUILD_DIR}/cloudbackup"

echo "############ [macos/${PKG_ARCH}] staging ############"
install -d "${STAGE}/usr/local/bin"
install -m 0755 "${BUILD_DIR}/cloudbackup" "${STAGE}/usr/local/bin/cloudbackup"

# Only the .sample is packaged; postinstall seeds config.yaml from it so an
# edited config is never part of an upgrade's file set.
install -d "${STAGE}/usr/local/etc/cloudbackup"
install -m 0644 packaging/macos/config.yaml.sample \
    "${STAGE}/usr/local/etc/cloudbackup/config.yaml.sample"

# Same webstatic subset the other platforms ship (see build-in-container.sh).
WEBROOT="${STAGE}/usr/local/share/cloudbackup/webstatic"
install -d "${WEBROOT}/ui"
cp -R webstatic/docs "${WEBROOT}/docs"
cp -R webstatic/docs_api "${WEBROOT}/docs_api"
cp webstatic/ui/index.html webstatic/ui/styles.css "${WEBROOT}/ui/"
cp -R webstatic/ui/js "${WEBROOT}/ui/js"
cp -R webstatic/ui/vendor "${WEBROOT}/ui/vendor"

# A macOS .pkg has no uninstall phase, so removal ships as a script.
install -m 0755 packaging/macos/uninstall.sh \
    "${STAGE}/usr/local/share/cloudbackup/uninstall.sh"

install -d "${STAGE}/usr/local/share/doc/cloudbackup"
install -m 0644 README.md "${STAGE}/usr/local/share/doc/cloudbackup/README.md"

install -d "${STAGE}/Library/LaunchDaemons"
install -m 0644 packaging/macos/cloudbackup.plist \
    "${STAGE}/Library/LaunchDaemons/${IDENTIFIER}.plist"

find "${STAGE}/usr/local/share/cloudbackup/webstatic" -type d -exec chmod 0755 {} +
find "${STAGE}/usr/local/share/cloudbackup/webstatic" -type f -exec chmod 0644 {} +

# pkgbuild wants the scripts named exactly preinstall / postinstall.
install -m 0755 packaging/macos/scripts/preinstall.sh  "${SCRIPTS}/preinstall"
install -m 0755 packaging/macos/scripts/postinstall.sh "${SCRIPTS}/postinstall"

echo "############ [macos/${PKG_ARCH}] running pkgbuild ############"
UNSIGNED="${PKG_DIR}/.cloudbackup_${PKG_VERSION}_macos_${PKG_ARCH}.unsigned.pkg"
OUT="${PKG_DIR}/cloudbackup_${PKG_VERSION}_macos_${PKG_ARCH}.pkg"
rm -f "${UNSIGNED}" "${OUT}"

# --ownership recommended makes the payload install as root:wheel regardless of
# who built it; CI runners build as an unprivileged user.
pkgbuild --root "${STAGE}" \
         --identifier "${IDENTIFIER}" \
         --version "${PKG_VERSION}" \
         --scripts "${SCRIPTS}" \
         --install-location / \
         --ownership recommended \
         "${UNSIGNED}"

if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
    echo "############ [macos/${PKG_ARCH}] signing ############"
    productsign --sign "${MACOS_SIGN_IDENTITY}" "${UNSIGNED}" "${OUT}"
    rm -f "${UNSIGNED}"
    pkgutil --check-signature "${OUT}"
else
    mv "${UNSIGNED}" "${OUT}"
    echo "notice: MACOS_SIGN_IDENTITY not set - shipping an UNSIGNED package."
    echo "notice: Gatekeeper will block double-click; install from a terminal with"
    echo "notice:   sudo installer -pkg <file> -target /"
fi

if [ -n "${MACOS_NOTARY_KEY_PATH:-}" ]; then
    if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
        echo "notarization requires signing; set MACOS_SIGN_IDENTITY too" >&2
        exit 1
    fi
    echo "############ [macos/${PKG_ARCH}] notarizing ############"
    xcrun notarytool submit "${OUT}" \
        --key "${MACOS_NOTARY_KEY_PATH}" \
        --key-id "${MACOS_NOTARY_KEY_ID}" \
        --issuer "${MACOS_NOTARY_KEY_ISSUER}" \
        --wait
    xcrun stapler staple "${OUT}"
    xcrun stapler validate "${OUT}"
fi

echo "############ [macos/${PKG_ARCH}] produced ${OUT} ############"
ls -lh "${OUT}"
echo "--- payload (first 15 entries) ---"
pkgutil --payload-files "${OUT}" | head -15
