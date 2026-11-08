BINARY:=wakeci
PWD:=$(shell pwd)
VERSION=0.0.0
VUE_VERSION_SUFFIX:=$(shell date +"%d%b")
MONOVA:=$(shell which monova dot 2> /dev/null)

export PATH := $(PATH):$(shell go env GOPATH)/bin

version:
ifdef MONOVA
override VERSION=$(shell monova)
else
	$(info "Install monova (https://github.com/jsnjack/monova) to calculate version")
endif

export VITE_VERSION = ${VERSION}-${VUE_VERSION_SUFFIX}

# go tool swag is pinned in go.mod to the swag library version docs.go imports
.ONESHELL:
src/backend/wakeci: version src/backend/*.go
	cd src/backend
	rm -rf assets
	cp -r ../frontend/dist/ assets
	mkdir -p docs
	touch docs/swagger.json
	go tool swag init --parseDependency --parseInternal --parseDepth 1 || exit 99
	CGO_ENABLED=0 go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}

.ONESHELL:
bin/wakeci: src/backend/wakeci
	cd src/backend
	cp wakeci ${PWD}/bin/

runf:
	cd src/frontend && npm run serve

runb: src/backend/wakeci
	cd src/backend
	ls *.go | entr -sr "cd ../../ && make src/backend/wakeci && ./src/backend/wakeci"

test_go:
	cd src/backend && go test ./...

test: test_go

E2E_ARGS ?=

testprod: build
	cd src/frontend && npm run test:prod -- $(E2E_ARGS) && echo "Open the HTML report from the repository root: make testreport"

testdev: build
	cd src/frontend && npm run test:dev -- $(E2E_ARGS)

testreport:
	cd src/frontend && npm run test:report -- $(E2E_ARGS)

vet:
	cd src/backend && go vet ./...

fmt:
	@command -v goimports >/dev/null 2>&1 || { \
	  echo "goimports is not installed. Install it with:"; \
	  echo "  go install golang.org/x/tools/cmd/goimports@latest"; \
	  exit 1; \
	}
	cd src/backend && goimports -w .

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 || { \
	  echo "golangci-lint is not installed. Install it with:"; \
	  echo "  grm install golangci/golangci-lint"; \
	  exit 1; \
	}
	cd src/backend && golangci-lint run

# Every agent session loads AGENTS.md whole, and Codex reads only its first
# 32 KiB: keep it under the limit (AGENTS.universal.md, "AGENTS.md size").
AGENTS_MAX_BYTES ?= 30000
agents-size:
	@size=$$(wc -c < AGENTS.md); \
	if [ "$$size" -gt $(AGENTS_MAX_BYTES) ]; then \
	  echo "AGENTS.md is $$size bytes, over the $(AGENTS_MAX_BYTES)-byte limit: move detail to docs/ (AGENTS.universal.md, \"AGENTS.md size\")"; \
	  exit 1; \
	fi

check: fmt vet build test lint testprod agents-size
	@echo "==> make check: all green"

standards:
	curl -sL https://raw.githubusercontent.com/jsnjack/standards/master/AGENTS.universal.md \
	    -o AGENTS.universal.md
	curl -sL https://raw.githubusercontent.com/jsnjack/standards/master/AGENTS.go.md \
	    -o AGENTS.go.md

buildf:
	cd src/frontend && npm run build

build: buildf bin/wakeci

# Releases only what passes the Go tests and the full E2E suite against the
# production build (testprod builds it)
release: test testprod
	grm release jsnjack/wakeci -f bin/${BINARY} -t "v`monova`"

.ONESHELL:
clean:
	rm -rf workdir/*
	rm -rf src/frontend/dist

clean_jobs:
	cd workdir && find . -name "*.yaml" -delete

.ONESHELL:
viewdb:
	cd workdir
	rm -f view.db
	cp wakeci.db view.db
	bolter -f view.db

.PHONY: runb runf version clean clean_jobs testprod testdev testreport test_go test vet fmt lint check agents-size standards buildf build release
