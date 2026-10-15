BINARY := vagrant-registry-export
BIN_DIR := bin
RELEASE_BIN_DIR := release
PKG := ./...

VERSION_PKG := github.com/hashicorp/vagrant-registry-export/version
GIT_COMMIT := $(shell git rev-parse HEAD)
GIT_DIRTY := $(if $(shell git status --porcelain),+CHANGES)
GIT_COMMIT_FLAG = $(VERSION_PKG).GitCommit=$(GIT_COMMIT)$(GIT_DIRTY)
BUILD_DATE ?= $(shell TZ=UTC0 git show -s --format=%cd --date=format-local:'%Y-%m-%dT%H:%M:%SZ' HEAD)
BUILD_DATE_FLAG = $(VERSION_PKG).BuildDate=$(BUILD_DATE)

GO_LDFLAGS = -X $(GIT_COMMIT_FLAG) -X $(BUILD_DATE_FLAG)

HOST_OS := $(shell go env GOOS)
HOST_ARCH := $(shell go env GOARCH)
OS ?= $(HOST_OS)
ARCH ?= $(HOST_ARCH)

.PHONY: build release-build release-builds run test test-coverage vet fmt fmt-check tidy clean

build: OUTPUT=$(BIN_DIR)/$(BINARY)## Build the CLI binary into bin/
build:
	@mkdir -p $(BIN_DIR)
	@rm -f "$(OUTPUT)"
	@echo "building -> $(OUTPUT)"
	@CGO_ENABLED=0 go build -ldflags "$(GO_LDFLAGS)" -o "$(OUTPUT)"

release-build: ## Build the CLI binary into bin/
	@rm -f $(RELEASE_BIN_DIR)/*
	@mkdir -p $(RELEASE_BIN_DIR)
	@for os in $(OS); do \
		for arch in $(ARCH); do \
			output="$(RELEASE_BIN_DIR)/$(BINARY)_$${os}_$${arch}"; \
			echo "building release $${os}/$${arch} -> $${output}"; \
			GOOS="$${os}" GOARCH="$${arch}" CGO_ENABLED=0 go build -ldflags "$(GO_LDFLAGS)" -o "$${output}" .; \
		done; \
	done

release-builds: ## Build common release targets into bin/
	$(MAKE) release-build OS="darwin linux windows" ARCH="amd64 arm64"

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
