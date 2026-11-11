# Developer entry points. Every target works offline once the module cache is warm.
# Tool versions are pinned here and in go.mod (tool directives).
GOLANGCI_LINT_VERSION ?= v2.15.0
GOVULNCHECK_VERSION   ?= v1.9.0

GO            ?= go
GOLANGCI_LINT ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK   ?= $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: all build test test-race examples lint vuln generate check-generate fmt tidy check-tidy docs manifest ci

all: build test lint

## build: compile every package, including the build-tagged examples
build:
	$(GO) build ./...
	$(GO) build -tags examples ./...

## test: offline unit tests (no Kentik credentials, no network)
test:
	$(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

## examples: compile the example tests without running them (they would call the live API)
examples:
	$(GO) test -tags examples -count=1 -run '^$$' ./examples/...

lint:
	$(GOLANGCI_LINT) run ./...

vuln:
	$(GOVULNCHECK) ./...

## generate: regenerate all generated code (stringer, service accessors, registry, manifest)
generate:
	$(GO) generate ./...

## check-generate: fail if generated files are stale
check-generate: generate
	git diff --exit-code

fmt:
	$(GO) tool gofumpt -w .

tidy:
	$(GO) mod tidy

check-tidy: tidy
	git diff --exit-code go.mod go.sum

## manifest: validate kentik-api.json
manifest:
	$(GO) run ./internal/cmd/manifestcheck kentik-api.json

## docs: serve package docs locally (pkg.go.dev style)
docs:
	$(GO) run golang.org/x/pkgsite/cmd/pkgsite@latest -open .

ci: build test examples lint vuln check-generate check-tidy manifest
