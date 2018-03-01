
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
GOASTCMD=$(GOPATH)/bin/gas
BINARY_NAME=cloudbackup
COVERAGE_FILE=coverage.out

# !!!! TABs MUST be tabs and not spaces; make does not like spaces instead of tabs !!!!
all: test build
build: 
	$(GOCMD) build -v
test:
	mkdir -p tmp config/tmp
	@echo "############ Running: go vet - checking for suspicious constructs ############"
	$(GOCMD) vet ./...
	@echo "############ Running: errcheck - checking unhandled errors ############"
	$(ERRCHKCMD) -verbose -abspath ./...
	@echo "############ Running: aligncheck - checking for inefficiently packed structs ############"
	find . -name "*.go" -not -path "./vendor/*" -not -name "*_test.go" -not -path "./config/config.go" -exec $(ALIGNCHECKCMD) {} \;
	@echo "############ Running: structcheck - checking for unused struct fields ############"
	$(STRUCTCHECKCMD) ./...
	@echo "############ Running: varcheck - checking for unused global variables and constants ############"
	$(VARCHECKCMD) ./...
	@echo "############ Running: gas - inspects source code for security problems by scanning the Go AST ############"
	$(GOASTCMD) ./...
	@echo "############ Running: go test - running unit tests ############"
	$(GOCMD) test -cover ./...
	@echo "############ Running: go test - running unit tests with race detection enabled ############"
	$(GOCMD) test -race -cover ./...
cover: 
	$(GOCMD) tool cover -html=$(COVERAGE_FILE)

clean: $(GOCMD) clean

run:
	$(GOCMD) build
	./$(BINARY_NAME)

deps: 
	$(GLIDECMD) install