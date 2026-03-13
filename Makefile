SERVICE_DIR := ./cmd/service
APP_DIR := ./cmd/everythingx
CLI_DIR := ./cmd/cli
E2E_TEST_DIR := ./e2eTest
TOOLS_DIR := ./tools
BIN_DIR := ./bin

OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')

VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X github.com/AlanKK/everythingx/internal/version.Version=${VERSION} -X github.com/AlanKK/everythingx/internal/version.Commit=${COMMIT} -X github.com/AlanKK/everythingx/internal/version.BuildDate=${BUILD_DATE}"

build: $(BIN_DIR)/everythingxd $(BIN_DIR)/everythingx $(BIN_DIR)/ev

$(BIN_DIR)/everythingxd: $(SERVICE_DIR)/*.go
	 go build ${LDFLAGS} -o $@ ./$(SERVICE_DIR)

$(BIN_DIR)/everythingx: $(APP_DIR)/*.go
	 CGO_LDFLAGS="-Wl,-w" go build ${LDFLAGS} -o $@ ./$(APP_DIR)

$(BIN_DIR)/ev: $(CLI_DIR)/main.go
	 go build ${LDFLAGS} -o $@ $<

e2e: build $(E2E_TEST_DIR)/e2etest
	$(E2E_TEST_DIR)/e2etest

$(E2E_TEST_DIR)/e2etest: $(E2E_TEST_DIR)/main.go
	 go build -o $@ $<

test:
	 go test ./... | grep -v "\[no test files\]"

ifeq ($(OS),darwin)
install: build app
	 sudo ./install.sh
else
install: build
	 sudo ./install-linux.sh
endif

clean:
	 rm -f $(BIN_DIR)/*
	 rm -f $(SERVICE_DIR)/everythingxd $(APP_DIR)/everythingx $(E2E_TEST_DIR)/e2etest
	 rm -f $(TOOLS_DIR)/check-missing-files/check-missing-files $(TOOLS_DIR)/create-db/create-db $(TOOLS_DIR)/scan-disk/scan-disk
	 rm -f $(TOOLS_DIR)/benchmark-db/benchmark-db $(TOOLS_DIR)/benchmark-db/benchmark.db 
	 rm -rf ./everythingx_test/* 
	 rm -f ./everythingx.zip ./everythingx-linux-*.tar.gz
	 rm -rf ./EverythingX.app

ifeq ($(OS),darwin)
uninstall:
	 sudo ./uninstall.sh
else
uninstall:
	 sudo ./uninstall-linux.sh
endif

ifeq ($(OS),darwin)
zip: build app
	 zip -r everythingx.zip bin/* install.sh uninstall.sh EverythingX.app

app: build
	 ~/go/bin/fyne package --release -os darwin -name EverythingX -icon assets/icons/retina/orange-black/folder-orange-black-512@2x.png -appID com.github.alankk.everythingx -executable $(BIN_DIR)/everythingx

pkg: app 
    #  --install-location "/" 
	pkgbuild \
     --root package/pkg \
     --identifier "com.github.alankk.pkg.EverythingX" \
     --version "alpha-1" \
     --install-location "/" \
	 --component-plist package/components.plist \
     --scripts package/scripts/postinstall \
     package/EverythingX.pkg
else
zip: build
	 tar czf everythingx-linux-amd64.tar.gz -C bin everythingxd ev && \
	   zip -j everythingx-linux-amd64.tar.gz install-linux.sh uninstall-linux.sh cmd/service/everythingxd.service

app:
	 ~/go/bin/fyne package --release -os linux -name EverythingX -icon assets/icons/retina/orange-black/folder-orange-black-512@2x.png -appID com.github.alankk.everythingx -executable $(BIN_DIR)/everythingx

deb: build
	 nfpm pkg --packager deb --target bin/

rpm: build
	 nfpm pkg --packager rpm --target bin/
endif

.PHONY: build test deps lint install clean uninstall zip package app pkg deb rpm
