
# Go parameters
GOCMD=go
GLIDECMD=glide
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
ERRCHKCMD=$(GOPATH)/bin/errcheck
ALIGNCHECKCMD=$(GOPATH)/bin/aligncheck
STRUCTCHECKCMD=$(GOPATH)/bin/structcheck
VARCHECKCMD=$(GOPATH)/bin/varcheck
GOASTCMD=$(GOPATH)/bin/gosec
BINARY_NAME=cloudbackup
COVERAGE_FILE=coverage.out

# !!!! TABs MUST be tabs and not spaces; make does not like spaces instead of tabs !!!!
all: test build
build:
	@$(GOCMD) version
	$(GOCMD) build -v
test: testcp gotest gotestrace
alltest: test build inttest
# test coding practices
testcp:
	@$(GOCMD) version
	@echo "############ Running: go fmt - ensure standard formatting ############"
	$(GOCMD) fmt ./...
	@echo "############ Running: go vet - checking for suspicious constructs ############"
	$(GOCMD) vet ./...
	@echo "############ Running: errcheck - checking unhandled errors ############"
	$(ERRCHKCMD) -verbose -abspath ./...
	@echo "############ Running: structcheck - checking for unused struct fields ############"
	$(STRUCTCHECKCMD) ./...
	@echo "############ Running: varcheck - checking for unused global variables and constants ############"
	$(VARCHECKCMD) ./...
	@echo "############ Running: gosec - inspects source code for security problems by scanning the Go AST ############"
	$(GOASTCMD) ./...
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
testdeps:
	$(GOCMD) get -u github.com/kisielk/errcheck
	$(GOCMD) install github.com/kisielk/errcheck
	$(GOCMD) get -u github.com/opennota/check/cmd/aligncheck
	$(GOCMD) install github.com/opennota/check/cmd/aligncheck
	$(GOCMD) get -u github.com/opennota/check/cmd/structcheck
	$(GOCMD) install github.com/opennota/check/cmd/structcheck
	$(GOCMD) get -u github.com/opennota/check/cmd/varcheck
	$(GOCMD) install github.com/opennota/check/cmd/varcheck
	$(GOCMD) get -u github.com/securego/gosec/cmd/gosec/...
	$(GOCMD) install github.com/securego/gosec/cmd/gosec

clean: $(GOCMD) clean

run:
	$(GOCMD) build
	./$(BINARY_NAME)

deps: 
	$(GLIDECMD) install

docs:
	@echo "############ Generating Documentation ############"
	./generate_docs.sh
