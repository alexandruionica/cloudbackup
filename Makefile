
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
else
	bash generate_version.sh
endif
	@$(GOCMD) version
	$(GOCMD) build -v -mod=vendor
test: testcp gotest gotestrace
alltest: test build inttest
# test coding practices
testcp:
	@$(GOCMD) version
	@echo "############ Running: go fmt - ensure standard formatting ############"
	$(GOCMD) fmt ./...
	@echo "############ Running: golangci-lint ############"
	$(GOLANGCILINTCMD) run --disable ineffassign --enable gosec --deadline=5m
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
	@echo "############ Generating Documentation ############"
	./generate_docs.sh
