# Variables
GO := go
BUILD_DIR := ./cmd
SERVICE_DIR := $(BUILD_DIR)/service
EVERYTHINGX_DIR := $(BUILD_DIR)/everythingx
CLI_DIR := $(BUILD_DIR)/cli
E2E_TEST_DIR := ./e2eTest
TOOLS_DIR := ./tools
OUTPUT_DIR := .

# Build targets
build: $(OUTPUT_DIR)/findfilesd $(OUTPUT_DIR)/everythingx $(OUTPUT_DIR)/ev

$(OUTPUT_DIR)/findfilesd: $(SERVICE_DIR)/main.go
	 $(GO) build -o $@ $<

$(OUTPUT_DIR)/everythingx: $(EVERYTHINGX_DIR)/*.go
	 CGO_LDFLAGS="-Wl,-w" $(GO) build -o $@ $^

$(OUTPUT_DIR)/ev: $(CLI_DIR)/main.go
	 $(GO) build -o $@ $<

all: build $(OUTPUT_DIR)/e2etest

$(OUTPUT_DIR)/e2etest: $(E2E_TEST_DIR)/main.go
	 $(GO) build -o $@ $<

# Test targets
test:
	 $(GO) test ./internal/ffdb
	 $(GO) test ./cmd/service

# Dependency management
deps:
	 $(GO) mod tidy

# Install target
install: build
	 sudo ./install.sh

# Clean target
clean:
	 rm -f $(OUTPUT_DIR)/*
	 rm -f $(SERVICE_DIR)/findfilesd $(EVERYTHINGX_DIR)/everythingx $(E2E_TEST_DIR)/e2etest
	 rm -f $(TOOLS_DIR)/check-missing-files/check-missing-files $(TOOLS_DIR)/create-db/create-db $(TOOLS_DIR)/scan-disk/scan-disk
	 rm -rf ./findfiles_test/*

# Uninstall target
uninstall:
	 sudo ./uninstall.sh

# Phony targets
.PHONY: build test deps lint install clean uninstall
