
# Go parameters
GOCMD=go
GLIDECMD=glide
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
BINARY_NAME=cloudbackup
COVERAGE_FILE=coverage.out

# !!!! TABs MUST be tabs and not spaces; make does not like spaces instead of tabs !!!!
all: test build
build: 
	$(GOCMD) build -v
test:
	$(GOCMD) vet ./...
	$(GOCMD) test -cover ./...

cover: 
	$(GOCMD) tool cover -html=$(COVERAGE_FILE)

clean: $(GOCMD) clean

run:
	$(GOCMD) build
	./$(BINARY_NAME)

deps: 
	$(GLIDECMD) install
