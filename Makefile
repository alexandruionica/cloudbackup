
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
# Pass DISTROS="deb12 el9" to limit; default is the full matrix.
packages:
	@echo "############ Building distribution packages ############"
	bash packaging/build-all.sh $(DISTROS)
