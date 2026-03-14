# EverythingX — Copilot Instructions

## Project Overview

EverythingX is a fast file-name search tool for macOS and Linux, inspired by [Everything by Voidtools](https://www.voidtools.com/support/everything/). It consists of three binaries:

| Binary | Source | Purpose |
|---|---|---|
| `everythingxd` | `cmd/service/` | Background daemon — indexes the filesystem into SQLite via FSEvents (macOS) or fanotify (Linux) |
| `everythingx` | `cmd/everythingx/` | GUI app — Fyne-based search interface |
| `ev` | `cmd/cli/` | CLI tool — fast command-line search |

## Tech Stack

- **Language**: Go 1.23+
- **GUI**: [Fyne v2](https://fyne.io/) (`fyne.io/fyne/v2`) with `github.com/dweymouth/fyne-tooltip`
- **Database**: SQLite3 via `github.com/mattn/go-sqlite3` (CGO required)
- **FS Events (macOS)**: `github.com/fsnotify/fsevents` (FSEvents API, `//go:build darwin`)
- **FS Events (Linux)**: `golang.org/x/sys/unix` fanotify with `FAN_MARK_FILESYSTEM` (`//go:build linux`, requires root + kernel 5.9+)
- **CLI flags**: `github.com/jessevdk/go-flags`

## Repository Layout

```
cmd/
  service/        # everythingxd daemon
    common.go           # shared daemon code (no build tag)
    main_darwin.go      # macOS FSEvents monitoring (//go:build darwin)
    main_linux.go       # Linux fanotify monitoring (//go:build linux)
    common_test.go      # platform-agnostic tests
    main_darwin_test.go # macOS-only tests
    main_linux_test.go  # Linux-only tests
    everythingxd.service # systemd unit file
    com.github.alankk.everythingxd.plist # launchd plist (macOS)
  everythingx/    # GUI app (main.go, ui.go, theme.go, assets.go, open_darwin.go, open_linux.go)
  cli/            # ev CLI tool
internal/
  ffdb/           # SQLite database package (all DB operations)
  shared/         # Models (SearchResult, EventRecord) and utilities
tools/            # Developer utilities (benchmarks, disk scan, etc.)
e2eTest/          # End-to-end tests
assets/           # App icons, screenshots, everythingx.desktop (Linux)
package/          # macOS .pkg installer assets + Linux nfpm scripts
bin/              # Compiled binaries (git-ignored)
nfpm.yaml         # .deb/.rpm packaging config (Linux)
install.sh        # macOS install script (launchd)
install-linux.sh  # Linux install script (systemd)
uninstall.sh      # macOS uninstall script
uninstall-linux.sh # Linux uninstall script
```

## Architecture

### Data Flow

1. **`everythingxd`** (service):
   - On startup, performs an initial full-disk scan and populates the SQLite DB.
   - **macOS**: subscribes to FSEvents for real-time create/delete notifications.
   - **Linux**: opens a fanotify fd with `FAN_MARK_FILESYSTEM` for mount-level monitoring.
   - Writes to the DB, which is opened in WAL mode to allow concurrent readers.
   - `shouldIgnorePath()` is defined per-platform: macOS skips `/System/Volumes/Data`; Linux skips `/proc`, `/sys`, `/run`, `/dev`, `/snap`.

2. **`everythingx` / `ev`** (consumers):
   - Open the DB **read-only** (`file:path?mode=ro`).
   - Execute prefix/substring searches via `ffdb.PrefixSearch`.

### Database Schema

```sql
CREATE TABLE files (
    filename    TEXT NOT NULL,
    fullpath    TEXT NOT NULL UNIQUE,
    event_id    INTEGER,
    object_type INTEGER
);
CREATE INDEX idx_filename ON files(filename COLLATE BINARY);
```

Default DB path: `/var/lib/everythingx/files.db`

### Search Query

```sql
SELECT fullpath, object_type
FROM files
WHERE filename LIKE ? COLLATE BINARY
ORDER BY filename ASC
LIMIT ?
```

The `%term%` wildcard is prepended by `PrefixSearch` in `internal/ffdb/ffdb.go`.

## Key Packages

### `internal/ffdb`

All database logic lives here:

- `CreateDB(pathname)` — creates a new DB with schema + indexes
- `OpenDB(pathname)` — opens for read/write (service)
- `OpenDBReadOnly(pathname)` — opens read-only (GUI/CLI)
- `PrefixSearch(prefix, limit)` — returns `[]*shared.SearchResult`
- `InsertRecord(record)` / `DeleteRecord(fullpath)` — called by the service

Prepared statements (`prefixSearchStmt`, `insertStmt`, `deleteStmt`) are package-level globals initialized on open.

### `internal/shared`

- **`SearchResult`** — `{Fullpath string, ObjectType ObjectType}`
- **`EventRecord`** — `{Filename, Path, ObjectType, EventID, EventTime, FoundOnScan}`
- **`ObjectType`** — `ItemIsFile`, `ItemIsDir`, `ItemIsSymlink`, etc.
- `FileExists(path)` — stat-based existence check
- `GetFileSizeMod(path)` — returns human-readable size + mod time
- `SplitFileName(filename, term)` — splits for bold-highlighting

### `cmd/everythingx` (GUI)

- Built with Fyne; uses `fyne.io/fyne/v2/widget.Table` for results.
- `handleAutoCompleteEntryChanged` debounces and triggers search on every keystroke.
- Double-click or Enter on a result opens the file's containing directory: `open -R` on macOS (`open_darwin.go`), `xdg-open` on Linux (`open_linux.go`).
- Theme switching supported via `theme.go`.
- Max results capped at `maxSearchResults = 1000`.

### `cmd/service` (daemon)

- Shared logic lives in `common.go` (no build tag): config parsing, DB setup, disk scan, event queue, DB writer goroutine.
- **macOS** (`main_darwin.go`, `//go:build darwin`): FSEvents stream monitoring; installed via launchd plist `com.github.alankk.everythingxd.plist`.
- **Linux** (`main_linux.go`, `//go:build linux`): fanotify mount-level monitoring via `golang.org/x/sys/unix`; `FAN_REPORT_DFID_NAME` (kernel 5.9+); installed as a systemd service `everythingxd.service`.
- Uses a channel (`dbChannel chan *shared.EventRecord`) to decouple FS events from DB writes.
- Handles `SIGTERM`/`SIGINT` for graceful shutdown.

## Build & Development

### Prerequisites

- Go 1.23+
- CGO toolchain (Xcode command-line tools on macOS; `gcc` on Linux)
- **Linux only**: `sudo apt-get install libgl1-mesa-dev xorg-dev` (required for Fyne/OpenGL)
- `fyne` CLI: `go install fyne.io/fyne/v2/cmd/fyne@latest` (macOS only, for `make app`)
- **Packaging**: `nfpm` for `.deb`/`.rpm` — `go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest`

### Common Commands

```bash
make build       # Build all three binaries into bin/
make test        # Run all unit tests
make e2e         # Build and run end-to-end tests
make install     # Build + install (requires sudo; uses launchd on macOS, systemd on Linux)
make app         # Package as EverythingX.app — macOS only
make pkg         # Build macOS .pkg installer
make deb         # Build .deb package — Linux only (requires nfpm)
make rpm         # Build .rpm package — Linux only (requires nfpm)
make zip         # Create distributable archive
make clean       # Remove build artifacts
```

### Running Locally

```bash
# Start the daemon (Full Disk Access on macOS; root required on Linux for fanotify)
sudo bin/everythingxd

# Search from CLI
bin/ev -b copilot-instructions.md

# Launch GUI
bin/everythingx
```

## Coding Conventions

- **Error handling**: most errors are logged and returned; fatal errors in DB setup use `log.Fatal`.
- **No global app state** outside of `ffdb` package-level prepared statements.
- **Context/cancellation**: the service uses OS signals rather than `context.Context`.
- **File size formatting**: use `shared.GetFileSizeMod` — do not duplicate size formatting logic.
- **Object type constants**: always use the `shared.ObjectType` constants (`ItemIsFile`, `ItemIsDir`, etc.), never raw integers.
- **DB access**: GUI and CLI must always use `OpenDBReadOnly`; only the service uses `OpenDB`/`CreateDB`.
- Tests live alongside source files (`_test.go` suffix). Run with `make test`.

## Platform Notes

### macOS
- Full Disk Access must be granted to `everythingxd` in System Preferences for complete indexing.
- The launchd plist installs to `/Library/LaunchDaemons/`.
- The GUI app is packaged as a standard macOS `.app` bundle using `fyne package`.
- `main_darwin.go` uses `//go:build darwin`; the `fsevents` package is darwin-only.

### Linux
- `main_linux.go` uses `//go:build linux`; fanotify requires **root** and **kernel 5.9+** for `FAN_REPORT_DFID_NAME`.
- The systemd service file installs to `/etc/systemd/system/everythingxd.service`.
- The data directory `/var/lib/everythingx/` is created automatically on first run if it doesn't exist.
- VS Code on Linux will show false-positive errors in `main_darwin.go` for `fsevents.*` symbols — these are a cross-compilation analysis artifact, not real build errors.
- Ignored paths: `/proc`, `/sys`, `/run`, `/dev`, `/snap`.

# everythingx Project Guide

## Coding Guidelines

### Code Simplicity Principles
- **Tests**: When writing and modifying tests, focus on the specific behavior being tested and avoid unnecessary changes.
- **Tests**: When writing and modifying tests, never modify the code being tested unless explicitly asked to do so.
- **Tests**: When writing unit tests, don't write tests to the behavior you see in the code. Look at function inputs/outputs and comments to determine purpose.
- **Do not add flexibility unless explicitly requested** — avoid adding code for hypothetical future use cases.
- **Do not add abstractions unless explicitly requested** — avoid adding layers of abstraction that are not needed.
- **Do not add complexity unless explicitly requested** — keep code simple and straightforward.
- **Do not add indirection unless explicitly requested** — avoid unnecessary layers of function calls.
- **Backwards and Legacy compatibility are forbidden** — do not add or keep code to maintain it.
- **Do not add optional parameters** unless they are explicitly requested or required by existing code.
- **Do not add fallback code** unless it is specifically requested.
- **Do not add features that are not asked for.**

### API Design
- Prefer required parameters over optional ones when the parameter is always needed.
- Prefer single-purpose functions over multi-purpose ones with many options.
- If all current usage follows one pattern, design the API for that pattern.
- Remove unused flexibility rather than keeping it "just in case".

### Development Practices
- Always clean up after yourself — delete temporary files, remove obsolete functions and imports.
- Keep function comments short and to the point. Only write comments in complex code.
- Don't write comments about refactoring or what's new/changed.
- Understand the Makefile before running build commands.
