SERVICE_DIR := ./cmd/service
APP_DIR := ./cmd/everythingx
CLI_DIR := ./cmd/cli
E2E_TEST_DIR := ./e2eTest
TOOLS_DIR := ./tools
BIN_DIR := ./bin

build: test $(BIN_DIR)/findfilesd $(BIN_DIR)/everythingx $(BIN_DIR)/ev

$(BIN_DIR)/findfilesd: $(SERVICE_DIR)/main.go
	 go build -o $@ $<

$(BIN_DIR)/everythingx: $(APP_DIR)/*.go
	 CGO_LDFLAGS="-Wl,-w" go build -o $@ $^

$(BIN_DIR)/ev: $(CLI_DIR)/main.go
	 go build -o $@ $<

all: build $(BIN_DIR)/e2etest

$(BIN_DIR)/e2etest: $(E2E_TEST_DIR)/main.go
	 go build -o $@ $<

test:
	 go test ./...

install: build
	 sudo ./install.sh

clean:
	 rm -f $(BIN_DIR)/*
	 rm -f $(BIN_DIR)/findfilesd $(BIN_DIR)/everythingx $(BIN_DIR)/ev $(BIN_DIR)/e2etest
	 rm -f $(SERVICE_DIR)/findfilesd $(APP_DIR)/everythingx $(E2E_TEST_DIR)/e2etest
	 rm -f $(TOOLS_DIR)/check-missing-files/check-missing-files $(TOOLS_DIR)/create-db/create-db $(TOOLS_DIR)/scan-disk/scan-disk
	 rm -rf ./findfiles_test/* ./everythingx.zip

uninstall:
	 sudo ./uninstall.sh
	
zip: build
	 zip -r everythingx.zip bin/*

.PHONY: build test deps lint install clean uninstall zip
