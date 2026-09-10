
# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOLANGCILINTCMD=golangci-lint
BINARY_NAME=cloudbackup
COVERAGE_FILE=coverage.out

# !!!! TABs MUST be tabs and not spaces; make does not like spaces instead of tabs !!!!
all: test build
build:
ifeq ($(OS),Windows_NT)
	@echo "Running on Windows"
	powershell '& .\generate_version.ps1'
else
	bash generate_version.sh
endif
	@$(GOCMD) version
	$(GOCMD) build -v -mod=vendor
test: testcp gotest gotestrace uitest
alltest: test inttest
uitest:
	@echo "############ Running: web UI unit tests ############"
ifeq ($(OS),Windows_NT)
	@echo "Running on Windows"
	powershell '& .\uitest.ps1'
else
	@echo "Running on some kind of Unix"
	./uitest.sh
endif
# Refresh webstatic/ui/vendor/ — the browser-side Preact/htm runtime the web UI
# loads instead of a CDN. Run after bumping a version in webstatic/ui/package.json
# and commit the result; "make uitest" verifies the committed copy is in sync.
uivendor:
	@echo "############ Vendoring web UI runtime dependencies ############"
ifeq ($(OS),Windows_NT)
	@echo "uivendor must be run from a Unix shell (it uses sha256sum/diff); the vendored files are committed, so Windows builds just consume them."
else
	cd webstatic/ui && npm install --no-audit --no-fund --silent && ./vendor-deps.sh
endif
# test coding practices
testcp:
	@$(GOCMD) version
	@echo "############ Running: go fmt - ensure standard formatting ############"
	$(GOCMD) fmt ./...
	@echo "############ Running: golangci-lint ############"
	$(GOLANGCILINTCMD) run --disable ineffassign --enable gosec
gotest:
ifeq ($(OS),Windows_NT)
	@echo "Running on Windows"
	IF NOT EXIST "c:\tmp" mkdir c:\tmp
	IF NOT EXIST tmp mkdir tmp
	IF NOT EXIST "config\tmp" mkdir config\tmp
else
	@echo "Running on some kind of Unix"
	mkdir -p tmp config/tmp
endif
	@echo "############ Running: go test - running unit tests ############"
	$(GOCMD) test -cover ./...
gotestrace:
ifeq ($(OS),Windows_NT)
	@echo "Running on Windows"
	IF NOT EXIST "c:\tmp" mkdir c:\tmp
	IF NOT EXIST tmp mkdir tmp
	IF NOT EXIST "config\tmp" mkdir config\tmp
else
	@echo "Running on some kind of Unix"
	mkdir -p tmp config/tmp
endif
	@echo "############ Running: go test - running unit tests with race detection enabled ############"
	$(GOCMD) test -race -cover ./...
inttest: build
	@echo "############ Running integration tests ############"
ifeq ($(OS),Windows_NT)
	@echo "Running on Windows"
	powershell '& .\integration_tests.ps1'
else
	@echo "Running on some kind of Unix"
	./integration_tests.sh
endif
cover: 
	$(GOCMD) tool cover -html=$(COVERAGE_FILE)
clean: $(GOCMD) clean

run:
	$(GOCMD) build
	./$(BINARY_NAME)

deps: 
	go mod tidy
	go mod vendor

docs:
	@echo "############ Regenerating Documentation ############"
	./generate_docs.sh

# Install Desloppify (https://github.com/peteromallet/desloppify) into a
# dedicated Python virtualenv. Linux/macOS only; not needed on Windows.
DESLOPPIFY_VENV=.venv_desloppify
desloppify:
ifeq ($(OS),Windows_NT)
	@echo "Desloppify is not needed on Windows; skipping."
else
	@echo "############ Installing Desloppify ############"
	@if [ ! -x $(DESLOPPIFY_VENV)/bin/python ]; then \
		echo "Creating virtualenv $(DESLOPPIFY_VENV) ..."; \
		virtualenv -p python3 $(DESLOPPIFY_VENV); \
	fi
	$(DESLOPPIFY_VENV)/bin/pip install -q --upgrade "desloppify[full]"
	@echo "Desloppify installed: $(DESLOPPIFY_VENV)/bin/desloppify"
endif

# Run a fast Desloppify code-health scan (Go, CI profile, no scorecard).
# Installs Desloppify into the virtualenv first if it isn't already present.
deslop:
ifeq ($(OS),Windows_NT)
	@echo "Desloppify is not needed on Windows; skipping."
else
	@if [ ! -x $(DESLOPPIFY_VENV)/bin/desloppify ]; then \
		echo "Desloppify not found; installing ..."; \
		$(MAKE) desloppify; \
	fi
	$(DESLOPPIFY_VENV)/bin/desloppify --lang go scan --path . --profile ci --no-badge
endif

# Build .deb and .rpm packages for all supported distros via Docker.
# Pass DISTROS="deb12 el9" to limit distros (default: full matrix) and
# ARCHS="amd64 arm64" to pick architectures (default: host arch only; arm64
# on an x86_64 host builds via QEMU emulation).
packages:
	@echo "############ Building distribution packages ############"
	ARCHS="$(ARCHS)" bash packaging/build-all.sh $(DISTROS)

# Cross-compile the Windows ARM64 (aarch64) build locally on an amd64 host via
# the clang-based llvm-mingw toolchain in Docker, and assemble the same release
# zip the CI windows job ships (dist/cloudbackup_<version>_windows_arm64.zip).
# Unix host with Docker only — no emulation, so it is fast. This proves the code
# *builds* for Windows-on-ARM; runtime verification needs the windows-11-arm
# GitHub Actions runner (or real ARM Windows hardware).
winbuild-arm64:
ifeq ($(OS),Windows_NT)
	@echo "winbuild-arm64 cross-compiles from a Unix host with Docker; on Windows build natively or use the windows-11-arm GitHub Actions runner."
else
	bash packaging/windows/build-winarm64.sh
endif

# Build the native Windows installer (.msi) via the WiX Toolset v5.
# Must run on Windows (GitHub Actions windows-latest or the Vagrant windows2025
# VM). See packaging/windows/README.md for prerequisites.
winpackage:
ifeq ($(OS),Windows_NT)
	@echo "############ Building Windows MSI installer ############"
	powershell '& .\packaging\windows\build-msi.ps1'
else
	@echo "Windows MSI packages must be built on Windows (GitHub Actions windows-latest or the Vagrant windows2025 VM)."
endif

# Build the FreeBSD .pkg. Must run ON FreeBSD (the freebsd14 Vagrant VM, or the
# vmactions VM the release workflow spins up) — go-sqlite3 needs cgo, so there
# is no cross-compile path from a Linux host.
#
# NOTE: FreeBSD's make(1) is bmake and cannot parse this GNU Makefile. Invoke it
# as `gmake freebsdpackage` (pkg install gmake).
freebsdpackage:
ifeq ($(OS),Windows_NT)
	@echo "FreeBSD packages must be built on FreeBSD."
else
	@echo "############ Building FreeBSD package ############"
	sh packaging/freebsd/build-pkg.sh
endif
