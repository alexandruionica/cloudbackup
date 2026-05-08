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

mkdir -p "${ROOT}/dist/packages"

for spec in "${TARGETS[@]}"; do
    IFS='|' read -r BASE FAMILY TAG <<< "${spec}"
    IMAGE="cloudbackup-builder:${TAG}"

    echo
    echo "============================================================"
    echo "  Target: ${BASE}  family=${FAMILY}  tag=${TAG}"
    echo "============================================================"

    docker build \
        --build-arg "BASE_IMAGE=${BASE}" \
        --build-arg "PKG_FAMILY=${FAMILY}" \
        -t "${IMAGE}" \
        -f "${ROOT}/packaging/Dockerfile.builder" \
        "${ROOT}/packaging"

    docker run --rm \
        -v "${ROOT}:/src" \
        -e "PKG_FAMILY=${FAMILY}" \
        -e "DISTRO_TAG=${TAG}" \
        -e "PKG_RELEASE=${PKG_RELEASE:-1}" \
        --user "$(id -u):$(id -g)" \
        "${IMAGE}" \
        bash /src/packaging/build-in-container.sh
done

echo
echo "============================================================"
echo "  All packages produced in dist/packages/:"
echo "============================================================"
ls -lh "${ROOT}/dist/packages/"
