# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=xdocker
BINARY_UNIX=$(BINARY_NAME)_unix

all: test build

build:
	$(GOBUILD) -o $(BINARY_NAME) -v

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

deps:
	$(GOGET) github.com/joho/godotenv
	$(GOGET) gopkg.in/yaml.v2
	$(GOGET) golang.org/x/crypto/ssh
	$(GOGET) github.com/superisaac/FEEL.go
	$(GOGET) github.com/hashicorp/go-version

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

docker-build:
	docker run --rm -it -v "$(PWD)":/usr/src/myapp -w /usr/src/myapp golang:1.16 make build

# Installs the binary to /usr/local/bin/
install: build
	mv $(BINARY_NAME) /usr/local/bin/

# Updates Go modules
update:
	$(GOGET) -u
	$(GOMOD) tidy

.PHONY: all build test clean run deps build-linux docker-build install update