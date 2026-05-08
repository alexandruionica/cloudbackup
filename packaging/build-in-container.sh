#!/usr/bin/env bash
set -euo pipefail

: "${PKG_FAMILY:?PKG_FAMILY (deb|rpm) must be set}"
: "${DISTRO_TAG:?DISTRO_TAG must be set (e.g. deb12, ubuntu24.04, el9)}"

cd /src

PKG_VERSION="$(cat misc/version.txt)"
PKG_RELEASE="${PKG_RELEASE:-1}"

BUILD_DIR="dist/build/${DISTRO_TAG}"
PKG_DIR="dist/packages"
mkdir -p "${BUILD_DIR}" "${PKG_DIR}"

export GOCACHE=/tmp/gocache
export GOMODCACHE=/tmp/gomodcache
export CGO_ENABLED=1

echo "############ [${DISTRO_TAG}] generating version stamp ############"
bash generate_version.sh

echo "############ [${DISTRO_TAG}] building binary ############"
go version
go build -v -mod=vendor -o "${BUILD_DIR}/cloudbackup" .

echo "############ [${DISTRO_TAG}] staging webstatic ############"
WEBROOT="${BUILD_DIR}/webstatic"
rm -rf "${WEBROOT}"
mkdir -p "${WEBROOT}/ui"
cp -r webstatic/docs "${WEBROOT}/docs"
cp -r webstatic/docs_api "${WEBROOT}/docs_api"
cp webstatic/ui/index.html webstatic/ui/styles.css "${WEBROOT}/ui/"
cp -r webstatic/ui/js "${WEBROOT}/ui/js"

echo "############ [${DISTRO_TAG}] running nfpm (${PKG_FAMILY}) ############"
case "${PKG_FAMILY}" in
    deb)
        OUT_FILE="${PKG_DIR}/cloudbackup_${PKG_VERSION}-${PKG_RELEASE}~${DISTRO_TAG}_amd64.deb"
        ;;
    rpm)
        OUT_FILE="${PKG_DIR}/cloudbackup-${PKG_VERSION}-${PKG_RELEASE}.${DISTRO_TAG}.x86_64.rpm"
        ;;
    *)
        echo "unsupported PKG_FAMILY: ${PKG_FAMILY}" >&2
        exit 1
        ;;
esac

# nfpm does not expand env vars in contents.src / scripts paths, so render
# the YAML ourselves with sed. All paths in nfpm.yaml are absolute (/src/...)
# so the rendered file can live anywhere.
RENDERED_CFG="${BUILD_DIR}/nfpm.yaml"
sed -e "s|\${DISTRO_TAG}|${DISTRO_TAG}|g" \
    -e "s|\${PKG_VERSION}|${PKG_VERSION}|g" \
    -e "s|\${PKG_RELEASE}|${PKG_RELEASE}|g" \
    /src/packaging/nfpm.yaml > "${RENDERED_CFG}"

nfpm pkg \
    --packager "${PKG_FAMILY}" \
    --config "${RENDERED_CFG}" \
    --target "/src/${OUT_FILE}"

echo "############ [${DISTRO_TAG}] produced ${OUT_FILE} ############"
ls -lh "/src/${OUT_FILE}"
