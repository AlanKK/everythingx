# Variables
BINARY_NAME=findfilesd
INSTALL_PATH=/usr/local/bin
PLIST_PATH=/Users/alan/Documents/git/findfiles/cmd/service/com.example.findfiles.plist
PLIST_INSTALL_PATH=/Library/LaunchDaemons/com.example.findfiles.plist

# Build the Go binary
build:
    go build -o $(BINARY_NAME) ./cmd/tools/scan-disk

# Run tests
test:
    go test ./...

# Install the binary and plist
install: build
    sudo cp $(BINARY_NAME) $(INSTALL_PATH)
    sudo cp $(PLIST_PATH) $(PLIST_INSTALL_PATH)
    sudo launchctl load $(PLIST_INSTALL_PATH)

# Clean up build artifacts
clean:
    rm -f $(BINARY_NAME)

# Uninstall the binary and plist
uninstall:
    sudo launchctl unload $(PLIST_INSTALL_PATH)
    sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
    sudo rm -f $(PLIST_INSTALL_PATH)

.PHONY: build test install clean uninstall