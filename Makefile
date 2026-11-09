SHELL := /bin/bash
BIN := bin/muxrail
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.DEFAULT_GOAL := help
.PHONY: help build test vet fmt e2e check tap

help: ## Show this help
	@awk -F':.*## ' '/^[a-z0-9-]+:.*## /{printf "  make %-8s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build bin/muxrail (static, no cgo)
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/muxrail

test: ## Unit tests
	go test ./...

vet: ## go vet
	go vet ./...

fmt: ## Fail when a file is not gofmt-clean
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

e2e: build ## End-to-end checks through a real tmux client (needs tmux)
	MUXRAIL_BIN=$(CURDIR)/$(BIN) go test -tags e2e -count=1 ./e2e/...

check: vet fmt test ## Vet, gofmt and unit tests

tap: ## Point the Homebrew tap at a release: TAP_DIR=path make tap TAG=vX.Y.Z
	./scripts/bump-tap.sh $(TAG)
