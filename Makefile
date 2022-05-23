BINARY:=wakeci
PWD:=$(shell pwd)
VERSION=0.0.0
VUE_VERSION_SUFFIX:=$(shell date +"%d%b")
MONOVA:=$(shell which monova dot 2> /dev/null)

version:
ifdef MONOVA
override VERSION=$(shell monova)
else
	$(info "Install monova (https://github.com/jsnjack/monova) to calculate version")
endif

export VITE_VERSION = ${VERSION}-${VUE_VERSION_SUFFIX}

.ONESHELL:
src/backend/wakeci: version src/backend/*.go
	cd src/backend
	rm -rf assets
	cp -r ../frontend/dist/ assets
	mkdir -p docs
	touch docs/swagger.json
	~/go/bin/swag init --parseDependency --parseInternal --parseDepth 1
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
	cd src/backend && go test

testprod: test_go
	cd src/frontend && npm run test:prod

testdev: test_go
	cd src/frontend && npm run test:dev

buildf:
	cd src/frontend && npm run build

build: buildf bin/wakeci

release: build
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

.PHONY: runb runf version clean clean_jobs testprod testdev test_go
