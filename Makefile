# Variables

# Build the main binaries
build:
	go build -o findfilesd ./cmd/service/main.go
	CGO_LDFLAGS="-Wl,-w" go build -o everythingx ./cmd/everythingx/main.go ./cmd/everythingx/ui.go ./cmd/everythingx/utils.go ./cmd/everythingx/assets.go

all: build
	go build -o e2etest ./e2eTest/main.go

# Run tests
test:
	go test ./internal/ffdb
	go test ./cmd/service

install: build
	sudo ./install.sh
    
clean:
	rm -f findfilesd everythingx cmd/service/findfilesd cmd/everythingx/everythingx cmd/e2eTest/e2etest cmd/e2eTest/main
	rm -f ./tools/check-missing-files/check-missing-files ./tools/create-db/create-db ./tools/scan-disk/scan-disk
	rm -rf ./findfiles_test/*
	
uninstall:
	sudo ./uninstall.sh
    
.PHONY: build test install clean uninstall
