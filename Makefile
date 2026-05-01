SHELL := /bin/bash
.DEFAULT_GOAL := build
.PHONY: build test lint fmt fmt-check tools ci clean run cover

BIN_DIR := $(CURDIR)/bin
BIN := $(BIN_DIR)/disbug
CMD := ./cmd/disbug

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/disbug-io/disbug-cli/internal/cmd.version=$(VERSION) \
           -X github.com/disbug-io/disbug-cli/internal/cmd.commit=$(COMMIT) \
           -X github.com/disbug-io/disbug-cli/internal/cmd.date=$(DATE)

GO_TEST_FLAGS ?= -race -count=1
TEST_PKGS ?= ./...

TOOLS_DIR := $(CURDIR)/.tools
GOFUMPT := $(TOOLS_DIR)/gofumpt
GOIMPORTS := $(TOOLS_DIR)/goimports
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint
TOOLS_STAMP := $(TOOLS_DIR)/.versions
TOOLS_VERSION := gofumpt=v0.9.2;goimports=v0.44.0;golangci-lint=v2.11.4

build:
	@mkdir -p $(BIN_DIR)
	@go build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

run: build
	@$(BIN) $(ARGS)

clean:
	@rm -rf $(BIN_DIR) dist coverage.txt

test:
	@go test $(GO_TEST_FLAGS) $(TEST_PKGS)

cover:
	@go test $(GO_TEST_FLAGS) -coverprofile=coverage.txt $(TEST_PKGS)
	@go tool cover -func=coverage.txt | tail -1

tools:
	@mkdir -p $(TOOLS_DIR)
	@if [ -x "$(GOFUMPT)" ] && [ -x "$(GOIMPORTS)" ] && [ -x "$(GOLANGCI_LINT)" ] && [ "$$(cat $(TOOLS_STAMP) 2>/dev/null)" = "$(TOOLS_VERSION)" ]; then \
		echo "tools up to date"; \
	else \
		GOBIN=$(TOOLS_DIR) go install mvdan.cc/gofumpt@v0.9.2; \
		GOBIN=$(TOOLS_DIR) go install golang.org/x/tools/cmd/goimports@v0.44.0; \
		GOBIN=$(TOOLS_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4; \
		printf '%s\n' "$(TOOLS_VERSION)" > "$(TOOLS_STAMP)"; \
	fi

fmt: tools
	@$(GOIMPORTS) -local github.com/disbug-io/disbug-cli -w .
	@$(GOFUMPT) -w .

fmt-check: tools
	@$(GOIMPORTS) -local github.com/disbug-io/disbug-cli -w .
	@$(GOFUMPT) -w .
	@git diff --exit-code -- '*.go' go.mod go.sum

lint: tools
	@$(GOLANGCI_LINT) run

ci: fmt-check lint test
