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

export VUE_APP_VERSION = ${VERSION}-${VUE_VERSION_SUFFIX}

.ONESHELL:
src/backend/wakeci: version src/backend/*.go
	cd src/backend
	rm -rf assets
	cp -r ../frontend/dist/ assets
	CGO_ENABLED=0 go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}

.ONESHELL:
bin/wakeci: src/backend/wakeci
	cd src/backend
	cp wakeci ${PWD}/bin/

runf:
	cd src/frontend && npm run serve

runb: src/backend/wakeci
	./src/backend/wakeci

testprod:
	cd src/frontend && npm run test:prod

testdev:
	cd src/frontend && npm run test:dev

buildf:
	cd src/frontend && npm run build

build: buildf bin/wakeci

release: build
	grm release jsnjack/wakeci -f bin/${BINARY} -t "v`monova`"

.ONESHELL:
clean:
	rm -rf example_wd/*
	rm -rf src/frontend/dist

.ONESHELL:
viewdb:
	cd example_wd
	rm -f view.db
	cp wakeci.db view.db
	bolter -f view.db

.PHONY: runb runf version
