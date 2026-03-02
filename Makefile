BUILD_DIR = bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

.PHONY: all build build-http-tap build-http-tapd install uninstall clean test lint proto

all: build

build: build-http-tap build-http-tapd

build-http-tap:
	@echo "Building..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/http-tap .

build-http-tapd:
	@echo "Building http-tapd..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/http-tapd ./cmd/http-tapd/

install:
	@echo "Installing http-tap and http-tapd..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	mkdir -p "$$bin_dir"; \
	echo "Installing to $$bin_dir"; \
	go build $(LDFLAGS) -o "$$bin_dir/http-tap" . && \
	go build $(LDFLAGS) -o "$$bin_dir/http-tapd" ./cmd/http-tapd/

uninstall:
	@echo "Uninstalling http-tap and http-tapd..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	rm -f "$$bin_dir/http-tap" "$$bin_dir/http-tapd"

proto:
	@command -v buf >/dev/null 2>&1 || { \
		echo "buf is not installed: https://buf.build/docs/installation"; \
		exit 1; \
	}
	buf generate

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

test:
	go test -race ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint is not installed"; \
		exit 1; \
	}
	golangci-lint run
