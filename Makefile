BINARY_NAME=shiptag
INSTALL_DIR=$(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-s -w -X main.version=$(VERSION)

.PHONY: all build install test vet fmt-check clean demo

all: build

build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/shiptag

install: build
	mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)/$(BINARY_NAME)"

test:
	go test -v ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files above need formatting"; exit 1; }

# Re-record docs/assets/demo.gif and dry-run.png (needs https://github.com/charmbracelet/vhs).
demo: build
	vhs docs/demo/demo.tape

clean:
	rm -rf bin/
