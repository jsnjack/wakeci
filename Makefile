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
	go tool swag init --parseInternal --parseDepth 1 || exit 99
	CGO_ENABLED=0 go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}

.ONESHELL:
bin/wakeci: src/backend/wakeci
	cd src/backend
	cp wakeci ${PWD}/bin/

# Starts the frontend and the backend in one terminal; open
# http://localhost:8080. The frontend dev server updates the page as frontend
# files change and passes API requests to the backend on 8081, which runb
# rebuilds and restarts. Ctrl-C stops both
run:
	$(MAKE) --no-print-directory -j2 runf runb RUNF_ARGS="--clearScreen false"

RUNF_ARGS ?=

runf:
	cd src/frontend && npm run serve -- $(RUNF_ARGS)

# The backend embeds a build of the frontend, so it needs one to compile; in
# development the frontend dev server serves the current files instead
src/backend/assets/index.html:
	test -f src/frontend/dist/index.html || (cd src/frontend && npm run build) || exit 1
	rm -rf src/backend/assets
	cp -r src/frontend/dist/ src/backend/assets

# Rebuilds the backend and restarts it when a Go file (tests aside), llms.txt,
# go.mod, Wakefile.yaml or the secrets file changes. entr stops when a file is
# added to a watched directory (-d, status 2) or a watched file is deleted
# (status 1), so the loop starts it again with the files there are now; it
# ends after Ctrl-C (status 0). The binary goes to bin/ and the API spec to
# docs/, made before entr starts, so a build adds nothing to a watched
# directory, which would restart it in a loop
runb: src/backend/assets/index.html
	cd src/backend
	mkdir -p docs && touch docs/swagger.json
	while :; do
		{ ls *.go | grep -v '_test\.go$$'; ls llms.txt go.mod go.sum ../../Wakefile.yaml ../../secrets.yaml 2>/dev/null; } |
			entr -dnrs 'go tool swag init --quiet --parseInternal --parseDepth 1 && go build -o ../../bin/wakeci-dev . && cd ../.. && exec bin/wakeci-dev'
		[ $$? -eq 0 ] && break
		sleep 0.5
	done

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

# Checks the frontend without changing it; `npm run lint:fix` and
# `npm run format` fix what they report
lintf:
	cd src/frontend && npm run lint && npm run format:check

# Every agent session loads AGENTS.md whole, and Codex reads only its first
# 32 KiB: keep it under the limit (AGENTS.universal.md, "AGENTS.md size").
AGENTS_MAX_BYTES ?= 30000
agents-size:
	@size=$$(wc -c < AGENTS.md); \
	if [ "$$size" -gt $(AGENTS_MAX_BYTES) ]; then \
	  echo "AGENTS.md is $$size bytes, over the $(AGENTS_MAX_BYTES)-byte limit: move detail to docs/ (AGENTS.universal.md, \"AGENTS.md size\")"; \
	  exit 1; \
	fi

check: fmt vet build test lint lintf testprod agents-size
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

.PHONY: run runb runf version clean clean_jobs testprod testdev testreport test_go test vet fmt lint lintf check agents-size standards buildf build release
