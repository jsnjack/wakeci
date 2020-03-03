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
	rm -f rice-box.go
	go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}

.ONESHELL:
bin/wakeci: version src/backend/*.go
	cd src/backend
	rm -f rice-box.go
	rice embed-go || exit 1
	go build -ldflags="-X main.Version=${VERSION}" -o ${BINARY}
	mv wakeci ${PWD}/bin/

runf:
	cd src/frontend && npm run serve

runb: src/backend/wakeci
	./src/backend/wakeci

test:
	cd src/frontend && npm run test

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

release: build
	python ~/lxdfs/cobro/ci/utils/release_on_github.py -f bin/${BINARY} -r jsnjack/wakeci -t "v`monova`"

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
