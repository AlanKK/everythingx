# Variables

# Build the Go binary
build:
	go build -o findfilesd ./cmd/service/main.go
	go build -o ff ./cmd/findfiles/main.go ./cmd/findfiles/ui.go ./cmd/findfiles/utils.go
	go build -o e2etest ./e2eTest/main.go

# Run tests
test:
	go test ./internal/ffdb

# Install the binary and plist
install: build
    
# Clean up build artifacts
clean:
	rm -f findfilesd ff

# Uninstall the binary and plist
uninstall:
    
.PHONY: build test install clean uninstall
