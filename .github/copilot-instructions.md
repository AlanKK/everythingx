# EverythingX — Copilot Instructions

## Project Overview

EverythingX is a fast file-name search tool for macOS (and Linux), inspired by [Everything by Voidtools](https://www.voidtools.com/support/everything/). It consists of three binaries:

| Binary | Source | Purpose |
|---|---|---|
| `everythingxd` | `cmd/service/` | Background daemon — indexes the filesystem into SQLite via FSEvents |
| `everythingx` | `cmd/everythingx/` | GUI app — Fyne-based search interface |
| `ev` | `cmd/cli/` | CLI tool — fast command-line search |

## Tech Stack

- **Language**: Go 1.23+
- **GUI**: [Fyne v2](https://fyne.io/) (`fyne.io/fyne/v2`) with `github.com/dweymouth/fyne-tooltip`
- **Database**: SQLite3 via `github.com/mattn/go-sqlite3` (CGO required)
- **FS Events**: `github.com/fsnotify/fsevents` (macOS FSEvents API)
- **CLI flags**: `github.com/jessevdk/go-flags`

## Repository Layout

```
cmd/
  service/        # everythingxd daemon (darwin build tag)
  everythingx/    # GUI app (main.go, ui.go, theme.go, assets.go)
  cli/            # ev CLI tool
internal/
  ffdb/           # SQLite database package (all DB operations)
  shared/         # Models (SearchResult, EventRecord) and utilities
tools/            # Developer utilities (benchmarks, disk scan, etc.)
e2eTest/          # End-to-end tests
assets/           # App icons and screenshots
package/          # macOS .pkg installer assets
bin/              # Compiled binaries (git-ignored)
```

## Architecture

### Data Flow

1. **`everythingxd`** (service):
   - On startup, performs an initial full-disk scan and populates the SQLite DB.
   - Subscribes to FSEvents to receive real-time create/delete notifications.
   - Writes to the DB, which is opened in WAL mode to allow concurrent readers.

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
- Double-click or Enter on a result calls `open -R <path>` (Finder reveal).
- Theme switching supported via `theme.go`.
- Max results capped at `maxSearchResults = 1000`.

### `cmd/service` (daemon)

- **Build tag**: `//go:build darwin` — macOS only.
- Uses a channel (`dbChannel chan *shared.EventRecord`) to decouple FSEvents from DB writes.
- Handles `SIGTERM`/`SIGINT` for graceful shutdown.
- Installed as a launchd plist: `cmd/service/com.github.alankk.everythingxd.plist`

## Build & Development

### Prerequisites

- Go 1.23+
- CGO toolchain (Xcode command-line tools on macOS)
- `fyne` CLI: `go install fyne.io/fyne/v2/cmd/fyne@latest`

### Common Commands

```bash
make build       # Build all three binaries into bin/
make test        # Run all unit tests
make e2e         # Build and run end-to-end tests
make install     # Build + install (requires sudo)
make app         # Package as EverythingX.app (fyne package)
make pkg         # Build macOS .pkg installer
make zip         # Create distributable zip
make clean       # Remove build artifacts
```

The GUI binary requires the CGO flag override:
```bash
CGO_LDFLAGS="-Wl,-w" go build -o bin/everythingx ./cmd/everythingx/*.go
```

### Running Locally

```bash
# Start the daemon (needs disk access permission on macOS)
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

- `cmd/service` is **macOS-only** (`//go:build darwin`). FSEvents is not available on Linux.
- Full Disk Access must be granted to `everythingxd` in macOS System Preferences for complete indexing.
- The launchd plist installs to `/Library/LaunchDaemons/`.
- The GUI app is packaged as a standard macOS `.app` bundle using `fyne package`.
