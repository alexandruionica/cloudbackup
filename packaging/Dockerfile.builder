ARG BASE_IMAGE=debian:12
FROM ${BASE_IMAGE}

ARG PKG_FAMILY=deb
ARG GO_VERSION=1.25.0
ARG NFPM_VERSION=2.41.0

ENV DEBIAN_FRONTEND=noninteractive
ENV PATH=/usr/local/go/bin:/usr/local/bin:${PATH}
ENV GOTOOLCHAIN=local

RUN set -eux; \
    if [ "${PKG_FAMILY}" = "deb" ]; then \
        apt-get update; \
        apt-get install -y --no-install-recommends \
            build-essential ca-certificates curl tar git xz-utils; \
        rm -rf /var/lib/apt/lists/*; \
    else \
        dnf -y --allowerasing install \
            gcc gcc-c++ glibc-devel make ca-certificates curl tar git xz which; \
        dnf clean all; \
    fi

RUN set -eux; \
    arch=$(uname -m); \
    case "${arch}" in \
        x86_64) goarch=amd64 ;; \
        aarch64) goarch=arm64 ;; \
        *) echo "unsupported arch: ${arch}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${goarch}.tar.gz" \
        | tar -C /usr/local -xz; \
    /usr/local/go/bin/go version

RUN set -eux; \
    arch=$(uname -m); \
    case "${arch}" in \
        x86_64) nfpm_arch=x86_64 ;; \
        aarch64) nfpm_arch=arm64 ;; \
        *) echo "unsupported arch: ${arch}" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/nfpm_${NFPM_VERSION}_Linux_${nfpm_arch}.tar.gz" \
        | tar -C /usr/local/bin -xz nfpm; \
    nfpm --version

WORKDIR /src
