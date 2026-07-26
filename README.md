# ![EverythingX icon](assets/icons/retina/white-orange/folder-white-orange-32@2x.png) EverythingX

## Overview
EverythingX is a _blazing_ fast file name search tool for macOS and Linux.

EverythingX attempts to replicate the very excellent Windows utility called [Everything by Voidtools](https://www.voidtools.com/support/everything/).

A background service captures real-time updates as files and directories change without scanning your disk. An app and a command-line tool do fast searches as you type.

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
| `everythingxd` | Background daemon — subscribes to filesystem updates in real time |
| `everythingx` | GUI application — instant search as you type |
| `ev` | CLI tool — fast command-line search |

## Command Line Interface (CLI)
The EverythingX CLI, called `ev`, allows you to search for files and directories from the command-line. It is far faster than using `find`. Pipe the output to grep or other tools to filter results.

### Usage
```
ev search_term [-b]

-b bold search term in the result. This option helps readability of the output but interferes with piping results to another command.
```

```
ev bashrc
/private/etc/bashrc
/private/etc/bashrc_Apple_Terminal
/home/alan/.bashrc
```

## EverythingX App
`everythingx` is a GUI application that provides an intuitive way to search and manage files on your system. Instant search results as you type to find full file paths and details.

## Background Service
The `everythingxd` daemon subscribes to filesystem updates from the kernel. It will occasionally index of your files to ensure fast and accurate search results.

### Features
- **Automatic indexing**: Keeps your file index up-to-date in real time.
- **Low resource usage**: Optimized to run efficiently in the background.
- **macOS**: Uses FSEvents for real-time filesystem monitoring.
- **Linux**: Uses fanotify (requires root and kernel 5.9+) for mount-level filesystem monitoring.

## Installation

### Quick install (recommended)

One command for both macOS and Linux — it detects your OS and CPU, downloads the
matching native package from the latest [release](https://github.com/AlanKK/everythingx/releases/latest),
and installs it (prompting for `sudo`):

```bash
curl -fsSL https://raw.githubusercontent.com/AlanKK/everythingx/main/install.sh | sh
```

Pin a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/AlanKK/everythingx/main/install.sh | EVERYTHINGX_VERSION=v0.2.2-beta sh
```

**Supported platforms:** macOS (Apple Silicon and Intel) and mainstream glibc
Linux with `systemd`, on `x86_64` and `arm64` — Debian/Ubuntu and derivatives via
`.deb`, Fedora/RHEL/openSUSE and derivatives via `.rpm`. Alpine/musl is not
supported.

### Manual download

Prefer to install by hand? Grab the package for your platform from the
[Releases](https://github.com/AlanKK/everythingx/releases/latest) page:

| Platform | Package |
|---|---|
| macOS (Apple Silicon) | `EverythingX_macos-apple-arm64.pkg` |
| macOS (Intel) | `EverythingX_macos-intel-amd64.pkg` |
| Linux x86_64 (Debian/Ubuntu) | `everythingx_linux_amd64.deb` |
| Linux x86_64 (Fedora/RHEL) | `everythingx_linux_amd64.rpm` |
| Linux arm64 (Debian/Ubuntu) | `everythingx_linux_arm64.deb` |
| Linux arm64 (Fedora/RHEL) | `everythingx_linux_arm64.rpm` |

**macOS:** double-click the `.pkg` file to install. Grant Full Disk Access to
`everythingxd` in `System Settings -> Privacy & Security -> Full Disk Access` for
complete indexing.

**Linux:**
```bash
# Debian/Ubuntu
sudo dpkg -i everythingx_linux_*.deb

# Fedora/RHEL
sudo dnf install ./everythingx_linux_*.rpm
```

The daemon requires root for fanotify (kernel 5.9+). The data directory `/var/lib/everythingx/` is created automatically. The GUI needs libGL/X libraries at runtime; on a minimal/headless server the `everythingxd` daemon and `ev` CLI still work without them.

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
