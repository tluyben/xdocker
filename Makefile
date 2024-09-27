# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=xdocker
BINARY_UNIX=$(BINARY_NAME)_unix
GLOBAL_EXTENSIONS_DIR=/usr/local/share/xdocker/extensions
GLOBAL_SERVICES_DIR=/usr/local/share/xdocker/services

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

# Installs the binary to /usr/local/bin/ and sets up the global extensions directory
install: build
	sudo mv $(BINARY_NAME) /usr/local/bin/
	sudo mkdir -p $(GLOBAL_EXTENSIONS_DIR)
	sudo chmod 755 $(GLOBAL_EXTENSIONS_DIR)
	if [ -d "./extensions" ]; then \
		sudo cp -R ./extensions/* $(GLOBAL_EXTENSIONS_DIR)/; \
	fi
	sudo mkdir -p $(GLOBAL_SERVICES_DIR)
	sudo chmod 755 $(GLOBAL_SERVICES_DIR)
	if [ -d "./services" ]; then \
		sudo cp -R ./services/* $(GLOBAL_SERVICES_DIR)/; \
	fi 

# Updates Go modules
update:
	$(GOGET) -u
	$(GOMOD) tidy

.PHONY: all build test clean run deps build-linux docker-build install update