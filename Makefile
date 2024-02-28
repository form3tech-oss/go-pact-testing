.DEFAULT_GOAL := default

GOLANGCI_VERSION := 1.56.2

platform := $(shell uname)
pact_version := "1.88.51"

GOFMT_FILES?=$$(find ./ -name '*.go' | grep -v vendor)

default: lint build test

ifeq (${platform},Darwin)
pact_filename := "pact-${pact_version}-osx.tar.gz"
else
pact_filename := "pact-${pact_version}-linux-x86_64.tar.gz"
endif

.PHONY: build
build: install-pact-go
	go install ./pacttesting

.PHONY: test
test:
	@echo "executing tests..."
	@go test -count=1 -v github.com/form3tech-oss/go-pact-testing/v2/pacttesting

install-pact-go:
	@if [ ! -d ./pact ]; then \
		echo "pact-go not installed, installing..."; \
		wget https://github.com/pact-foundation/pact-ruby-standalone/releases/download/v${pact_version}/${pact_filename} -O /tmp/pactserver.tar.gz && tar -xvf /tmp/pactserver.tar.gz -C .; \
	fi

.PHONY: lint
lint: tools/golangci-lint
	@echo "==> Running golangci-lint..."
	@tools/golangci-lint run

.PHONY: lint-fix
lint-fix: tools/golangci-lint
	@echo "==> Running golangci-lint with autofix..."
	@tools/golangci-lint run --fix

.PHONY: tools/golangci-lint
tools/golangci-lint:
	@echo "==> Installing golangci-lint..."
	@./scripts/install-golangci-lint.sh $(GOLANGCI_VERSION)
