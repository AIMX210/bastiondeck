.PHONY: all build web agent cli test test-race vet fmt loc smoke clean tidy

GO ?= go
WEB_DIR := web

all: web build

## build: compile the daemon with embedded web UI
build:
	$(GO) build -o bin/bastiondeck ./cmd/bastiondeck

## agent: compile the standalone reverse agent
agent:
	cd agent && $(GO) build -o ../bin/bd-agent ./cmd/bd-agent

## cli: compile the native CLI
cli:
	$(GO) build -o bin/bdk ./cmd/bdk

## web: type-check, test and build the React UI into internal/webui/dist
web:
	cd $(WEB_DIR) && npm install --no-audit --no-fund && npm run build

web-test:
	cd $(WEB_DIR) && npx vitest run

## test: full Go test suite (server module + agent module)
test:
	$(GO) test ./...
	cd agent && $(GO) test ./...

test-race:
	$(GO) test -race ./...
	cd agent && $(GO) test -race ./...

vet:
	$(GO) vet ./...
	cd agent && $(GO) vet ./...

fmt:
	$(GO) fmt ./...
	cd agent && $(GO) fmt ./...

tidy:
	$(GO) mod tidy
	cd agent && $(GO) mod tidy

## loc: report source lines of code; --check enforces the 20k floor
loc:
	python3 scripts/loc.py

loc-check:
	python3 scripts/loc.py --check

## smoke: end-to-end smoke against an in-process fake SSH fleet
smoke:
	python3 scripts/smoke.py

clean:
	rm -rf bin
