# Project paths
SERVICE_DIR := ./cmd/service
APP_DIR := ./cmd/everythingx
CLI_DIR := ./cmd/cli
E2E_TEST_DIR := ./e2eTest
TOOLS_DIR := ./tools
BIN_DIR := ./bin

# Output binaries
SERVICE_BIN := $(BIN_DIR)/everythingxd
APP_BIN := $(BIN_DIR)/everythingx
CLI_BIN := $(BIN_DIR)/ev
E2E_BIN := $(E2E_TEST_DIR)/e2etest

# Tools
GO := go
FYNE := $(shell go env GOPATH)/bin/fyne
NFPM := $(shell go env GOPATH)/bin/nfpm

TARGET_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')

# App/package metadata
APP_NAME := EverythingX
APP_ID := com.github.alankk.everythingx
APP_ICON := assets/icons/retina/orange-black/folder-orange-black-512@2x.png
PKG_VERSION := beta-1
PKG_IDENTIFIER := com.github.alankk.pkg.EverythingX
PKG_OUTPUT := EverythingX.pkg

# Build metadata
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X github.com/AlanKK/everythingx/internal/version.Version=$(VERSION) -X github.com/AlanKK/everythingx/internal/version.Commit=$(COMMIT) -X github.com/AlanKK/everythingx/internal/version.BuildDate=$(BUILD_DATE)"
TAGS := -tags fts5

# Build targets
build: $(SERVICE_BIN) $(APP_BIN) $(CLI_BIN)

$(SERVICE_BIN): $(SERVICE_DIR)/*.go
	$(GO) build $(TAGS) $(LDFLAGS) -o $@ ./$(SERVICE_DIR)

$(APP_BIN): $(APP_DIR)/*.go
	CGO_LDFLAGS="-Wl,-w" $(GO) build $(TAGS) $(LDFLAGS) -o $@ ./$(APP_DIR)

$(CLI_BIN): $(CLI_DIR)/main.go
	$(GO) build $(TAGS) $(LDFLAGS) -o $@ $<

$(E2E_BIN): $(E2E_TEST_DIR)/main.go
	$(GO) build $(TAGS) -o $@ $<

# Test targets
ifeq ($(TARGET_OS),linux)
e2e: build $(E2E_BIN)
	sudo $(E2E_BIN)
else
e2e: build $(E2E_BIN)
	$(E2E_BIN)
endif

test:
	$(GO) test $(TAGS) ./... | grep -v "\[no test files\]"

# Install targets
ifeq ($(TARGET_OS),darwin)
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
	rm -f ./EverythingX.pkg
	rm -rf ./EverythingX.app

ifeq ($(TARGET_OS),darwin)
uninstall:
	sudo ./uninstall.sh
else
uninstall:
	sudo ./uninstall-linux.sh
endif

# Packaging targets
app: build
	$(FYNE) package --release -os $(TARGET_OS) -name $(APP_NAME) -icon $(APP_ICON) -appID $(APP_ID) -executable $(APP_BIN)

ifeq ($(TARGET_OS),darwin)
zip: build app
	zip -r everythingx.zip $(BIN_DIR)/* install.sh uninstall.sh EverythingX.app

pkg: app
	pkgbuild \
		--component EverythingX.app \
		--identifier $(PKG_IDENTIFIER) \
		--version $(PKG_VERSION) \
		--install-location /Applications \
		$(PKG_OUTPUT)
else
zip: build
	tar czf everythingx-linux-amd64.tar.gz -C $(BIN_DIR) everythingxd ev && \
	zip -j everythingx-linux-amd64.tar.gz install-linux.sh uninstall-linux.sh cmd/service/everythingxd.service

deb: build
	$(NFPM) pkg --packager deb --target $(BIN_DIR)/

rpm: build
	$(NFPM) pkg --packager rpm --target $(BIN_DIR)/
endif

.PHONY: build test deps lint install clean uninstall zip package app pkg deb rpm
