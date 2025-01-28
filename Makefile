# Variables

# Build the Go binary
build:
	go build -o findfilesd ./cmd/service/main.go
	CGO_LDFLAGS="-Wl,-w" go build -o ff ./cmd/findfiles/main.go ./cmd/findfiles/ui.go ./cmd/findfiles/utils.go ./cmd/findfiles/assets.go
	go build -o e2etest ./e2eTest/main.go

# Run tests
test:
	go test ./internal/ffdb
	go test ./cmd/service

# Install the binary and plist
install: build
    
# Clean up build artifacts
clean:
	rm -f findfilesd ff cmd/service/findfilesd cmd/findfiles/ff cmd/e2eTest/e2etest cmd/e2eTest/main
	rm -f ./tools/check-missing-files/check-missing-files ./tools/create-db/create-db ./tools/scan-disk/scan-disk
	rm -rf ./findfiles_test/*
	

# Uninstall the binary and plist
uninstall:
    
.PHONY: build test install clean uninstall
