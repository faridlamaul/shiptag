BINARY_NAME=shiptag
INSTALL_DIR=$(HOME)/.local/bin

.PHONY: all build install test clean

all: build

build:
	go build -ldflags="-s -w" -o bin/$(BINARY_NAME) ./cmd/shiptag

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"

test:
	go test -v ./...

vet:
	go vet ./...

clean:
	rm -rf bin/
