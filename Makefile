
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
alltest: test build inttest
uitest:
	@echo "############ Running: web UI unit tests ############"
	@node_major=$$(node -p "process.versions.node.split('.')[0]" 2>/dev/null || echo 0); \
	if [ "$$node_major" -lt 18 ]; then \
		echo "ERROR: web UI tests require Node.js >= 18 (found $$(node --version 2>/dev/null || echo none))."; \
		echo "       If using nvm: 'nvm use 20' (or newer) before running make."; \
		exit 1; \
	fi
	cd webstatic/ui && npm test
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
inttest:
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

# Build .deb and .rpm packages for all supported distros via Docker.
# Pass DISTROS="deb12 el9" to limit; default is the full matrix.
packages:
	@echo "############ Building distribution packages ############"
	bash packaging/build-all.sh $(DISTROS)
