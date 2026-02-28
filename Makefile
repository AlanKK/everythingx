SERVICE_DIR := ./cmd/service
APP_DIR := ./cmd/everythingx
CLI_DIR := ./cmd/cli
E2E_TEST_DIR := ./e2eTest
TOOLS_DIR := ./tools
BIN_DIR := ./bin

VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X github.com/AlanKK/everythingx/internal/version.Version=${VERSION} -X github.com/AlanKK/everythingx/internal/version.Commit=${COMMIT} -X github.com/AlanKK/everythingx/internal/version.BuildDate=${BUILD_DATE}"

build: $(BIN_DIR)/everythingxd $(BIN_DIR)/everythingx $(BIN_DIR)/ev

$(BIN_DIR)/everythingxd: $(SERVICE_DIR)/main.go
	 go build ${LDFLAGS} -o $@ $<

$(BIN_DIR)/everythingx: $(APP_DIR)/*.go
	 CGO_LDFLAGS="-Wl,-w" go build ${LDFLAGS} -o $@ $^

$(BIN_DIR)/ev: $(CLI_DIR)/main.go
	 go build ${LDFLAGS} -o $@ $<

e2e: build $(E2E_TEST_DIR)/e2etest
	$(E2E_TEST_DIR)/e2etest

$(E2E_TEST_DIR)/e2etest: $(E2E_TEST_DIR)/main.go
	 go build -o $@ $<

test:
	 go test ./... | grep -v "\[no test files\]"

bench: tools/benchmark-db/benchmark.db
	 go test -bench=. -benchtime=5s -count=3 ./tools/benchmark-db/

tools/benchmark-db/benchmark.db:
	 cd tools/benchmark-db && go run . -n 100
install: build app
	 sudo ./install.sh

clean:
	 rm -f $(BIN_DIR)/*
	 rm -f $(SERVICE_DIR)/everythingxd $(APP_DIR)/everythingx $(E2E_TEST_DIR)/e2etest
	 rm -f $(TOOLS_DIR)/check-missing-files/check-missing-files $(TOOLS_DIR)/create-db/create-db $(TOOLS_DIR)/scan-disk/scan-disk
	 rm -f $(TOOLS_DIR)/benchmark-db/benchmark-db $(TOOLS_DIR)/benchmark-db/benchmark.db $(TOOLS_DIR)/benchmark-db/benchmark.idx
	 rm -rf ./everythingx_test/* 
	 rm -f ./everythingx.zip
	 rm -rf ./EverythingX.app

uninstall:
	 sudo ./uninstall.sh
	
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

.PHONY: build test bench deps lint install clean uninstall zip package app pkg
