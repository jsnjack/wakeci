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
export GOPATH = ${PWD}:${GOPATH}

.ONESHELL:
src/backend/wakeci: version src/backend/*.go
	cd src/backend
	dep ensure
	rm -f rice-box.go
	go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}

.ONESHEL:
bin/wakeci: version src/backend/*.go
	go get github.com/golang/dep/cmd/dep || exit 1
	go get github.com/GeertJohan/go.rice/rice || exit 1
	cd src/backend
	dep ensure
	rm -f rice-box.go
	rice embed-go
	go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}
	mv wakeci ${PWD}/bin/

runf:
	cd src/frontend && npm run serve

runb: src/backend/wakeci
	./src/backend/wakeci -wdir example_wd/ -cdir example_cd/

buildf:
	cd src/frontend && npm run build

build: buildf bin/wakeci

deploy: build
	ssh wakeci mkdir wakedir
	ssh wakeci mkdir wakeconfig
	ssh wakeci sudo systemctl stop ${BINARY} || exit 0
	ssh wakeci rm -f ${BINARY}
	scp bin/${BINARY} wakeci:~/
	ssh wakeci sudo setcap cap_net_bind_service=+ep ${BINARY}
	ssh wakeci sudo systemctl start ${BINARY}
	ssh wakeci sudo systemctl status ${BINARY}

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
