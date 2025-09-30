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
