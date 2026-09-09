BINARY := vagrant-registry-export
BIN_DIR := bin
PKG    := ./...
HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)
OS ?= $(HOST_OS)
ARCH ?= $(HOST_ARCH)

.PHONY: build build-all run test test-coverage vet fmt fmt-check tidy clean

build: ## Build the CLI binary into bin/
	mkdir -p $(BIN_DIR)
	@for os in $(OS); do \
		for arch in $(ARCH); do \
			output="$(BIN_DIR)/$(BINARY)_$${os}_$${arch}"; \
			if [ "$${os}" = "$(HOST_OS)" ] && [ "$${arch}" = "$(HOST_ARCH)" ] && [ "$(words $(OS))" -eq 1 ] && [ "$(words $(ARCH))" -eq 1 ]; then \
				output="$(BIN_DIR)/$(BINARY)"; \
			fi; \
			echo "building $${os}/$${arch} -> $${output}"; \
			GOOS="$${os}" GOARCH="$${arch}" CGO_ENABLED=0 go build -o "$${output}" .; \
		done; \
	done

build-all: ## Build common release targets into bin/
	$(MAKE) build OS="darwin linux windows" ARCH="amd64 arm64"

run: ## Run the CLI (use ARGS="..." to pass arguments)
	go run . $(ARGS)

test: ## Run tests
	go test $(PKG)

test-coverage: ## Run tests with coverage
	go test $(PKG) -coverprofile=coverage.out -covermode=atomic

vet: ## Run go vet
	go vet $(PKG)

fmt: ## Format code
	gofmt -l -w .

fmt-check: ## Check formatting
	@test -z "$$(gofmt -l .)"

tidy: ## Tidy go.mod/go.sum
	go mod tidy

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
