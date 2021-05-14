.DEFAULT_GOAL := default

platform := $(shell uname)
pact_version := "1.88.51"

GOFMT_FILES?=$$(find ./ -name '*.go' | grep -v vendor)

default: build test


ifeq (${platform},Darwin)
pact_filename := "pact-${pact_version}-osx.tar.gz"
else
pact_filename := "pact-${pact_version}-linux-x86_64.tar.gz"
endif

build: goimportscheck vet install-pact-go
	go install ./pacttesting

install-pact-go:
	@if [ ! -d ./pact ]; then \
		echo "pact-go not installed, installing..."; \
		wget https://github.com/pact-foundation/pact-ruby-standalone/releases/download/v${pact_version}/${pact_filename} -O /tmp/pactserver.tar.gz && tar -xvf /tmp/pactserver.tar.gz -C .; \
	fi

install-goimports:
	@go get golang.org/x/tools/cmd/goimports

test: goimportscheck
	@echo "executing tests..."
	@go test -v github.com/form3tech-oss/go-pact-testing/pacttesting

vet:
	@echo "go vet ."
	@go vet $$(go list ./... | grep -v vendor/) ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

goimports:
goimports:
	goimports -w $(GOFMT_FILES)

goimportscheck:
	@sh -c "'$(CURDIR)/scripts/goimportscheck.sh'"


.PHONY: build test vet goimports goimportscheck errcheck package
