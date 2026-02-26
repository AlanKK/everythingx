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
# everythingx Project Guide

## What is everythingx?
EverythingX is a blazing fast file name search tool for macOS and Linux that replicates the functionality of the Windows "Everything" utility. It consists of a background service that maintains a real-time file index, a GUI application for interactive searching, and a CLI tool for command-line file searches. The system provides minimal resource usage with quick indexing, real-time updates, and instant search results.

## Tech Stack
- **Language**: Go 1.24+
- **GUI Framework**: Fyne v2.6.0+ for cross-platform desktop application
- **Database**: SQLite3 (mattn/go-sqlite3) for file indexing and search
- **File System Events**: fsevents (macOS+linux) for real-time file system monitoring
- **CLI Parsing**: jessevdk/go-flags for command-line argument handling
- **Tooltips**: dweymouth/fyne-tooltip for enhanced UI experience
- **Build System**: Make for build automation
- **Packaging**: Fyne packaging tools for macOS app bundle creation

## Project Structure
```
everythingx/
├── cmd/                          # Main applications
│   ├── cli/                      # Command-line interface (ev)
│   ├── everythingx/             # GUI application
│   └── service/                 # Background indexing service (everythingxd)
├── internal/                    # Internal packages
│   ├── ffdb/                    # File database operations (SQLite)
│   └── shared/                  # Shared models and utilities
├── tools/                       # Development and testing tools
├── assets/                      # Application icons and resources
├── bin/                         # Compiled binaries
├── EverythingX.app/            # macOS application bundle
├── e2eTest/                    # End-to-end testing
└── data/                       # Runtime data directory

## Development Workflow

### Backend Development
Always work from the `backend/` directory unless modifying and testing hedygrpc.

```bash
make format      # Fix code formatting
```

## Key Development Commands


### Testing Strategy
- **Unit tests**: Fast, isolated, mock external dependencies
- **Integration tests**: Real gRPC + mock server, sqlite database (set `RUN_INTEGRATION_TESTS=true`)


## Architecture Patterns

### Multi-Component Architecture
The system follows a three-component architecture:
1. **Background Service** (`everythingxd`): Monitors file system events and maintains SQLite database
2. **GUI Application** (`everythingx`): Fyne-based desktop app for interactive file searching
3. **CLI Tool** (`ev`): Command-line interface for scriptable file searches

### Database-Centric Design
- **SQLite3 Database**: Central file index stored at `/var/lib/everythingx/files.db`
- **Prepared Statements**: Pre-compiled SQL statements for optimal search performance
- **Prefix Search**: Efficient file name searching using database indexes
- **Real-time Updates**: File system events trigger database updates

### Event-Driven File Monitoring
- **FSEvents Integration**: macOS and Linux file system event monitoring for real-time updates
- **Channel-Based Processing**: Go channels for async event processing between file monitor and database
- **Selective Monitoring**: Ignores system paths like `/System/Volumes/Data` to reduce noise

### Modular Package Structure
- **Internal Packages**: `ffdb` for database operations, `shared` for common models
- **Separation of Concerns**: Database logic, UI logic, and CLI logic in separate modules
- **Shared Models**: Common data structures (`SearchResult`, `EventRecord`, `ObjectType`) across components

### Cross-Platform GUI with Fyne
- **Native Look**: Fyne provides native appearance on macOS and Linux
- **Responsive UI**: Real-time search results with table-based display
- **Theme Support**: Built-in light/dark theme switching
- **Desktop Integration**: File opening and Finder integration on macOS

## Coding Guidelines

### Code Simplicity Principles
- **Tests**: When writing and modifying tests, focus on the specific behavior being tested and avoid unnecessary changes.
- **Tests**: When writing and modifying tests, never modify the code being tested unless explicitly asked to do so.
- **Tests**: When writing unit tests, don't write tests to the behavior you see in the code. Look at function inputs/outputs and comments to determine purpose
- **Do not add flexibility unless explicitly requested** - avoid adding code for hypothetical future use cases
- **Do not add abstractions unless explicitly requested** - avoid adding layers of abstraction that are not needed
- **Do not add complexity unless explicitly requested** - keep code simple and straightforward
- **Do not add indirection unless explicitly requested** - avoid unnecessary layers of function calls or classes
- **Backwards and Legacy compatibility are forbidden** - do not add or keep code to maintain it
- **Do not add Union types** unless there are actual, documented use cases for multiple types
- **Do not add optional parameters** unless they are explicitly requested or required by existing code
- **Do not add string alternatives** to enum parameters unless there's a clear need for dynamic/configurable behavior
- **Do not add "convenience" overloads** that accept multiple input formats when one format is sufficient
- **Do not add fallback code** unless it is specifically requested
- **Do not add features that are not asked for**
- **Do not add code to support multiple versions of a dependency** unless it is asked for

### API Design
- **Prefer required parameters over optional ones** when the parameter is always needed
- **Prefer single-purpose functions** over multi-purpose ones with many options
- **If all current usage follows one pattern, design the API for that pattern**
- **Look at actual usage patterns** before adding flexibility
- **If no callers need string input, don't accept string input**
- **If no callers pass None, don't make parameters optional**
- **Remove unused flexibility** rather than keeping it "just in case"
- **Write code for today's requirements, not hypothetical future ones**

### Development Practices
- Always use `uv` for all Python-related tasks and environments
- When testing changes, run `make test`, then `make test-all` from backend/ directory and then find the broken test.
- Always clean up after yourself - delete temporary files, remove obsolete functions and imports
- Keep function comments short and to the point
- Don't write comments about refactoring or what's new/changed
- Only write comments in complex code
- Understand the Makefile
