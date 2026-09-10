#!/bin/sh
# Install the pinned Go toolchain under /usr/local/go and symlink it into PATH.
#
# The FreeBSD "go" package tracks whatever version the ports tree currently
# carries, which is not necessarily the one this project is built and tested
# with, so CI pulls the official freebsd-amd64 tarball instead -- the same
# thing the freebsd14 Vagrant VM does. Keep GO_VERSION in step with the
# GO_VERSION constant at the top of the Vagrantfile.
#
# No-op when the pinned version is already installed, so it is cheap to call
# from a VM that has been provisioned already.
set -eu

GO_VERSION="${GO_VERSION:-1.26.8}"
TARBALL="go${GO_VERSION}.freebsd-amd64.tar.gz"

if [ -x /usr/local/go/bin/go ] && \
   /usr/local/go/bin/go version | grep -q "go${GO_VERSION} "; then
    echo "Go ${GO_VERSION} already installed"
else
    echo "############ Installing Go ${GO_VERSION} ############"
    # fetch(1) is in base but needs the ca_root_nss package for HTTPS.
    fetch -o "/tmp/${TARBALL}" "https://dl.google.com/go/${TARBALL}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${TARBALL}"
    rm -f "/tmp/${TARBALL}"
fi

ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

go version
