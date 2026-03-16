# EverythingX

## Overview
EverythingX is a _blazing_ fast file name search tool for macOS and Linux.

EverythingX attempts to replicate the very excellent Windows utility called [Everything by Voidtools](https://www.voidtools.com/support/everything/).

A background service maintains real-time updates as files and directories change. An app and a command-line tool do fast searches as you type.

## How is EverythingX different from other search tools such as Spotlight, locate, and find?
- Minimal resource usage
- Quick file indexing
- Real-time updating
- Clean and simple user interface
- Instant search results as you type
- Quick startup
- Open source

## Components

| Binary | Purpose |
|--------|---------|
| `everythingxd` | Background daemon — indexes the filesystem in real time |
| `everythingx` | GUI application — instant search as you type |
| `ev` | CLI tool — fast command-line search |

## Command Line Interface (CLI)
The EverythingX CLI, called `ev`, allows you to search the database for files and directories from the command-line. It is far faster than using `find` but has fewer options. Pipe the output to grep or other tools to filter results.

### Usage
```
ev search_term [-b]

-b bold search term in the result. This option helps readability of the output but interferes with piping results to another command.
```

```
ev -b bashrc
/private/etc/bashrc
/private/etc/bashrc_Apple_Terminal
/home/alan/.bashrc
```

## EverythingX App
`everythingx` is a GUI application that provides an intuitive way to search and manage files on your system. Instant search results as you type to find full file paths and details.

## Background Service
The `everythingxd` daemon continuously indexes your files to ensure fast and accurate search results.

### Features
- **Automatic indexing**: Keeps your file index up-to-date in real time.
- **Low resource usage**: Optimized to run efficiently in the background.
- **macOS**: Uses FSEvents for real-time filesystem monitoring.
- **Linux**: Uses fanotify (requires root and kernel 5.9+) for mount-level filesystem monitoring.

## Installation

### macOS
```bash
make build
sudo make install   # installs via launchd
```
Full Disk Access must be granted to `everythingxd` in System Preferences for complete indexing.

### Linux
```bash
# Install build dependencies (first time only)
sudo apt-get install libgl1-mesa-dev xorg-dev gcc

make build
sudo make install   # installs via systemd
```

The daemon requires root to use fanotify. The data directory `/var/lib/everythingx/` is created automatically on first run.

### Linux packages (.deb / .rpm)
Pre-built packages are available on the [Releases](https://github.com/AlanKK/everythingx/releases) page, or build them yourself:
```bash
make deb   # requires nfpm
make rpm   # requires nfpm
```

## Running Locally (without installing)
```bash
make build

# Start the daemon
sudo bin/everythingxd

# Search from CLI
bin/ev -b filename

# Launch GUI
bin/everythingx
```

## Building from Source

### Prerequisites
- Go 1.23+
- CGO toolchain (Xcode CLT on macOS; `gcc` on Linux)
- **Linux GUI**: `sudo apt-get install libgl1-mesa-dev xorg-dev`
- **macOS app bundle**: `go install fyne.io/fyne/v2/cmd/fyne@latest`
- **Linux packages**: `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`

### Build Commands
```bash
make build    # Build all binaries into bin/
make test     # Run unit tests
make app      # Package EverythingX.app (macOS only)
make deb      # Build .deb package (Linux only)
make rpm      # Build .rpm package (Linux only)
make clean    # Remove build artifacts
```

## License
EverythingX is licensed under the MIT License. See the [LICENSE](LICENSE) file for more information.

## Contact, feature requests, and bug reports
Create an issue on the [Github Page](https://github.com/AlanKK/everythingx/issues)
