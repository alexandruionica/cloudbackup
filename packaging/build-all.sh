#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

# Each spec: <base_image>|<pkg_family>|<distro_tag>
TARGETS=(
    "debian:11|deb|deb11"
    "debian:12|deb|deb12"
    "ubuntu:22.04|deb|ubuntu22.04"
    "ubuntu:24.04|deb|ubuntu24.04"
    "rockylinux:8|rpm|el8"
    "rockylinux:9|rpm|el9"
)

# Architectures to build (Go/nfpm names). Default: host arch only, so a plain
# `make packages` stays fast and needs no emulation. Opt into arm64 (or both)
# with ARCHS="amd64 arm64"; on an x86_64 host arm64 builds run under QEMU.
case "$(uname -m)" in
    x86_64)  HOST_GOARCH=amd64 ;;
    aarch64) HOST_GOARCH=arm64 ;;
    *)       HOST_GOARCH=amd64 ;;
esac
ARCHS="${ARCHS:-${HOST_GOARCH}}"

# Allow filtering: ./build-all.sh deb12 el9
if [ "$#" -gt 0 ]; then
    FILTERED=()
    for want in "$@"; do
        for spec in "${TARGETS[@]}"; do
            tag="${spec##*|}"
            if [ "${tag}" = "${want}" ]; then
                FILTERED+=("${spec}")
            fi
        done
    done
    if [ "${#FILTERED[@]}" -eq 0 ]; then
        echo "no matching targets for: $*" >&2
        echo "available tags: $(printf '%s ' "${TARGETS[@]##*|}")" >&2
        exit 1
    fi
    TARGETS=("${FILTERED[@]}")
fi

if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required but not found in PATH" >&2
    exit 1
fi

# If a non-host arch is requested, the target container runs under QEMU. Fail
# early with a fix-it hint if the binfmt handler isn't registered. binfmt
# handlers are named by QEMU's arch (aarch64), not the Go name (arm64).
for a in ${ARCHS}; do
    [ "${a}" = "${HOST_GOARCH}" ] && continue
    case "${a}" in
        amd64) qemu_arch=x86_64 ;;
        arm64) qemu_arch=aarch64 ;;
        *)     qemu_arch="${a}" ;;
    esac
    if [ ! -e "/proc/sys/fs/binfmt_misc/qemu-${qemu_arch}" ]; then
        echo "cross-arch build requested (${a}) but QEMU binfmt is not registered." >&2
        echo "register it with: docker run --privileged --rm tonistiigi/binfmt --install all" >&2
        exit 1
    fi
done

mkdir -p "${ROOT}/dist/packages"

for spec in "${TARGETS[@]}"; do
    IFS='|' read -r BASE FAMILY TAG <<< "${spec}"
    for ARCH in ${ARCHS}; do
        case "${ARCH}" in
            amd64) PLATFORM=linux/amd64 ;;
            arm64) PLATFORM=linux/arm64 ;;
            *) echo "unsupported ARCH: ${ARCH}" >&2; exit 1 ;;
        esac
        IMAGE="cloudbackup-builder:${TAG}-${ARCH}"

        echo
        echo "============================================================"
        echo "  Target: ${BASE}  family=${FAMILY}  tag=${TAG}  arch=${ARCH}"
        if [ "${ARCH}" != "${HOST_GOARCH}" ]; then
            echo "  (cross-arch build via QEMU emulation — expect it to be slow)"
        fi
        echo "============================================================"

        docker build \
            --platform "${PLATFORM}" \
            --build-arg "BASE_IMAGE=${BASE}" \
            --build-arg "PKG_FAMILY=${FAMILY}" \
            -t "${IMAGE}" \
            -f "${ROOT}/packaging/Dockerfile.builder" \
            "${ROOT}/packaging"

        docker run --rm \
            --platform "${PLATFORM}" \
            -v "${ROOT}:/src" \
            -e "PKG_FAMILY=${FAMILY}" \
            -e "DISTRO_TAG=${TAG}" \
            -e "PKG_RELEASE=${PKG_RELEASE:-1}" \
            --user "$(id -u):$(id -g)" \
            "${IMAGE}" \
            bash /src/packaging/build-in-container.sh
    done
done

echo
echo "============================================================"
echo "  All packages produced in dist/packages/:"
echo "============================================================"
ls -lh "${ROOT}/dist/packages/"
