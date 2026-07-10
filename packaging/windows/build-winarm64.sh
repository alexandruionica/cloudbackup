#!/usr/bin/env bash
# Cross-compile the Windows ARM64 (aarch64) build on an amd64 Unix host and
# assemble the same release zip the CI Windows job ships.
#
# Windows-on-ARM binaries can only be produced by the LLVM/clang-based
# llvm-mingw toolchain (the gcc mingw-w64 cannot target ARM64). We run that
# toolchain plus Go in Docker, so nothing beyond Docker is needed on the host.
# This is a real cross-compile (no emulation) and proves the code *builds* for
# Windows-on-ARM; actually running it needs real ARM Windows (e.g. the
# windows-11-arm GitHub Actions runner).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${ROOT}"

if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required but not found in PATH" >&2
    exit 1
fi

IMAGE="cloudbackup-winarm-builder:latest"
BUILD_DIR="dist/build/windows-arm64"
STAGE="${BUILD_DIR}/cloudbackup"

echo "############ [win/arm64] building cross toolchain image ############"
docker build -t "${IMAGE}" \
    -f "${ROOT}/packaging/windows/Dockerfile.winarm-builder" \
    "${ROOT}/packaging/windows"

echo "############ [win/arm64] generating version stamp ############"
bash generate_version.sh
PKG_VERSION="$(tr -d '[:space:]' < misc/version.txt)"

echo "############ [win/arm64] cross-compiling cloudbackup.exe (windows/arm64) ############"
rm -rf "${BUILD_DIR}"
mkdir -p "${STAGE}"
# GOOS/GOARCH/CGO_ENABLED/CC are baked into the image. -buildvcs=false because
# git refuses to stamp inside the container (dubious ownership on the mount);
# generate_version.sh above already bakes the commit id into misc/version.go.
docker run --rm \
    -v "${ROOT}:/src" \
    -w /src \
    -e HOME=/tmp \
    -e GOCACHE=/tmp/gocache \
    -e GOMODCACHE=/tmp/gomodcache \
    --user "$(id -u):$(id -g)" \
    "${IMAGE}" \
    go build -v -mod=vendor -buildvcs=false -o "${STAGE}/cloudbackup.exe" .

echo "############ [win/arm64] staging release tree ############"
# Mirror the webstatic layout the CI windows-zip job ships (see
# .github/workflows/release.yml): docs, docs_api, and a trimmed ui.
WEBROOT="${STAGE}/webstatic"
mkdir -p "${WEBROOT}/ui"
cp -r webstatic/docs "${WEBROOT}/docs"
cp -r webstatic/docs_api "${WEBROOT}/docs_api"
cp webstatic/ui/index.html webstatic/ui/styles.css "${WEBROOT}/ui/"
cp -r webstatic/ui/js "${WEBROOT}/ui/js"
# Windows-specific sample config (Windows paths + service commands).
cp packaging/windows/config.yaml.sample "${STAGE}/config.yaml.sample"
cp README.md "${STAGE}/README.md"

ZIP="dist/cloudbackup_${PKG_VERSION}_windows_arm64.zip"
rm -f "${ZIP}"
# Zip with a top-level cloudbackup/ dir, matching the CI Compress-Archive output.
# python3 is already a project prerequisite (integration tests), so we avoid a
# hard dependency on the `zip` binary.
python3 - "${ZIP}" "${BUILD_DIR}" <<'PY'
import os, sys, zipfile
zippath, builddir = sys.argv[1], sys.argv[2]
root = os.path.join(builddir, "cloudbackup")
with zipfile.ZipFile(zippath, "w", zipfile.ZIP_DEFLATED) as z:
    for dp, _, files in os.walk(root):
        for f in files:
            full = os.path.join(dp, f)
            z.write(full, os.path.relpath(full, builddir))
PY

echo "############ [win/arm64] produced ${ZIP} ############"
ls -lh "${ZIP}"
if command -v file >/dev/null 2>&1; then
    file "${STAGE}/cloudbackup.exe"
fi
