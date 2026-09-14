#!/usr/bin/env bash
#
# Install smoke test for one Linux package: installs it into a fresh container of the distro it was
# built for, checks what the package promises (binary, service user, config and data dir ownership,
# web assets, systemd unit), starts the daemon exactly as the unit does, talks to it with the CLI
# client, stops it, removes the package and checks what must survive removal.
#
#   bash packaging/smoke-test.sh dist/packages/cloudbackup_0.0.3-1~deb12_amd64.deb debian:12
#   bash packaging/smoke-test.sh dist/packages/cloudbackup-0.0.3-1.el9.x86_64.rpm rockylinux:9 linux/amd64
#
# build-all.sh runs this after every package it produces (SMOKE=0 skips it).
set -euo pipefail

PKG="${1:?package file}"
BASE="${2:?base image, e.g. debian:12}"
PLATFORM="${3:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PKG_ABS="$(cd "$(dirname "${PKG}")" && pwd)/$(basename "${PKG}")"

platform_args=()
if [ -n "${PLATFORM}" ]; then
    platform_args=(--platform "${PLATFORM}")
fi

echo "############ smoke test: $(basename "${PKG}") on ${BASE} ############"
docker run --rm "${platform_args[@]}" \
    -v "${PKG_ABS}:/pkg/$(basename "${PKG}"):ro" \
    -v "${ROOT}/packaging/smoke-in-container.sh:/smoke.sh:ro" \
    "${BASE}" \
    bash /smoke.sh "/pkg/$(basename "${PKG}")"
