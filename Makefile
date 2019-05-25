BINARY:=wakeci
PWD:=$(shell pwd)
VERSION=0.0.0
MONOVA:=$(shell which monova dot 2> /dev/null)

version:
ifdef MONOVA
override VERSION="$(shell monova)"
else
	$(info "Install monova (https://github.com/jsnjack/monova) to calculate version")
endif

.ONESHELL:
src/backend/wakeci: version src/backend/*.go
	cd src/backend
	dep ensure
	go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}

runf:
	cd src/frontend && npm run serve

runb: src/backend/wakeci
	./src/backend/wakeci

.PHONY: runb runf version
