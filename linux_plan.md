# Plan: Add Full Linux Support to EverythingX

## TL;DR

Add Linux support by splitting the macOS-specific service daemon into platform-specific files using Go build tags, implementing Linux filesystem monitoring via **fanotify** (mount-level, matching FSEvents behavior), adding systemd service management, making the GUI/CLI cross-platform, creating .deb/.rpm packages, and adding a GitHub Actions workflow with tag-based releases.

---

## Phase 1: Split Service Daemon into Platform-Specific Files

The service daemon (`cmd/service/main.go`) is entirely `//go:build darwin` and tightly coupled to the FSEvents API. This needs to be broken apart.

### Step 1.1 — Create `cmd/service/common.go` (no build tag, shared code)

Extract from `cmd/service/main.go`:
- `Config` struct + `getCommandLineArgs()`
- Package variables: `dbChannel`, `fullPathLikeQuery`, `fileExists`, `verbose`, `config`
- `setupDatabase()`, `scanHomeDirs()`, `databaseWriter()`, `addEventToQueue()`, `deleteMissing()`
- `scanDisk()` — modify to call `shouldIgnorePath(path)` (defined per-platform) instead of the hardcoded `strings.HasPrefix(path, ignorePath)`

### Step 1.2 — Create `cmd/service/main_darwin.go` (`//go:build darwin`)

Move existing macOS code here:
- FSEvents imports (`github.com/fsnotify/fsevents`)
- `noteDescription` map
- `ignorePath` constant
- `shouldIgnorePath()` → `return strings.HasPrefix(path, "/System/Volumes/Data")`
- `main()` with FSEvents device/stream setup and event listener goroutine
- `buildEventRecord(*fsevents.Event)`
- `gracefulShutdown(db, es *fsevents.EventStream)`
- `setupSignalHandlers(db, es)`

### Step 1.3 — Create `cmd/service/main_linux.go` (`//go:build linux`, NEW)

New fanotify-based implementation using `golang.org/x/sys/unix`:
- `FanotifyInit` with `FAN_CLASS_NOTIF | FAN_REPORT_DFID_NAME` (kernel 5.9+)
- `FanotifyMark` with `FAN_MARK_ADD | FAN_MARK_FILESYSTEM` for mount-level monitoring
- Watch events: `FAN_CREATE | FAN_DELETE | FAN_MOVED_FROM | FAN_MOVED_TO | FAN_ONDIR`
- `shouldIgnorePath()` — ignore `/proc`, `/sys`, `/run`, `/dev`, `/snap`
- `main()` with fanotify FD setup, event read loop, signal handlers
- Event parsing: read `fanotify_event_metadata`, resolve paths via `/proc/self/fd/<fd>` + DFID_NAME info header for name
- `gracefulShutdown()` / `setupSignalHandlers()` — same signals, no EventStream reference

### Step 1.4 — Split test file

Current `cmd/service/main_test.go` has `//go:build darwin`:
- `TestDeleteMissing` → move to `cmd/service/common_test.go` (no build tag)
- `TestBuildEventRecord` → keep in `cmd/service/main_darwin_test.go`
- Create `cmd/service/main_linux_test.go` for Linux event handling tests

---

## Phase 2: Make GUI Cross-Platform

### Step 2.1 — Extract `handleOpenFile()` into platform-specific files

The `handleOpenFile()` function in `cmd/everythingx/ui.go` (line 39) uses `exec.Command("open", "-R", pathname)` which is macOS-only.

Create platform-specific implementations:
- `cmd/everythingx/open_darwin.go`: `exec.Command("open", "-R", pathname)` (existing behavior)
- `cmd/everythingx/open_linux.go`: `exec.Command("xdg-open", filepath.Dir(pathname))` (opens file manager to containing directory)

Remove `handleOpenFile` from `ui.go`.

No other GUI changes needed — Fyne, system tray, theming, and table widgets already work on Linux.

---

## Phase 3: Linux Service Management

### Step 3.1 — Create systemd service file

Create `cmd/service/everythingxd.service`:
```ini
[Unit]
Description=EverythingX File Index Daemon
After=local-fs.target

[Service]
Type=simple
ExecStart=/usr/local/bin/everythingxd --monitor_path=/ --db_path=/var/lib/everythingx/files.db
Restart=always
StandardOutput=append:/var/log/everythingxd.log
StandardError=append:/var/log/everythingxd.log

[Install]
WantedBy=multi-user.target
```

### Step 3.2 — Create Linux install/uninstall scripts

**`install-linux.sh`**:
- Copy `everythingxd`, `ev` to `/usr/local/bin/`
- Create `/var/lib/everythingx/` data directory
- Copy `everythingxd.service` to `/etc/systemd/system/`
- `systemctl daemon-reload && systemctl enable --now everythingxd`

**`uninstall-linux.sh`**:
- `systemctl stop everythingxd && systemctl disable everythingxd`
- Remove service file, binaries, data directory

---

## Phase 4: Build System Updates

### Step 4.1 — Update Makefile

- Make `build` target work on both platforms (Go cross-compilation handles this with build tags)
- Fix `app` target: detect OS, use `-os linux` on Linux, `-os darwin` on macOS
- Add `deb` target: build `.deb` package using `nfpm`
- Add `rpm` target: build `.rpm` package using `nfpm`
- Update `install` target: run `install-linux.sh` on Linux, `install.sh` on macOS
- Update `zip` target: include appropriate service config per OS
- Fix `install.sh` hardcoded path (`/Users/alan/Documents/git/everythingx/`) → use relative path

### Step 4.2 — Add `nfpm` configuration for .deb/.rpm

Create `nfpm.yaml` at repo root:
- Defines package metadata, files to include, systemd service, pre/post install scripts
- Single config generates both `.deb` and `.rpm`

---

## Phase 5: GitHub Actions

### Step 5.1 — Update `.github/workflows/main.yml`

Add Linux matrix entries:
```yaml
matrix:
  include:
    - os: macos-latest
      arch: arm64
    - os: macos-15-intel
      arch: amd64
    - os: ubuntu-latest
      arch: amd64
```

For all runners: `make test`, `make build`.
For macOS: e2e test, upload macOS artifacts.
For Linux: e2e test, upload Linux artifacts.
Install Fyne's Linux build deps: `libgl1-mesa-dev`, `xorg-dev`, `libxcursor-dev`, `libxrandr-dev`, `libxinerama-dev`, `libxi-dev`, `libxxf86vm-dev`.

### Step 5.2 — Add release job triggered on tag push (`v*`)

On tag push:
- Build all platforms (macOS arm64/amd64, Linux amd64)
- Package: macOS zip, Linux `.deb` + `.rpm`
- Create GitHub Release with all artifacts attached

---

## Files to Modify

| File | Change |
|------|--------|
| `cmd/service/main.go` | Split into `common.go` + `main_darwin.go` |
| `cmd/service/main_test.go` | Split into `common_test.go` + `main_darwin_test.go` |
| `cmd/everythingx/ui.go` | Extract `handleOpenFile` to platform-specific files |
| `Makefile` | Add Linux targets, OS detection, fix hardcoded paths |
| `.github/workflows/main.yml` | Add Linux CI + release on tags |
| `install.sh` | Fix hardcoded `/Users/alan/...` path → use relative path |
| `go.mod` | Promote `golang.org/x/sys` to direct dependency |

## New Files to Create

| File | Purpose |
|------|---------|
| `cmd/service/common.go` | Shared daemon code (no build tag) |
| `cmd/service/main_darwin.go` | macOS FSEvents monitoring (`//go:build darwin`) |
| `cmd/service/main_linux.go` | Linux fanotify monitoring (`//go:build linux`) |
| `cmd/service/common_test.go` | Platform-agnostic unit tests |
| `cmd/service/main_darwin_test.go` | macOS-specific tests (FSEvents) |
| `cmd/service/main_linux_test.go` | Linux-specific tests (fanotify) |
| `cmd/service/everythingxd.service` | systemd unit file |
| `cmd/everythingx/open_darwin.go` | macOS file opening (`open -R`) |
| `cmd/everythingx/open_linux.go` | Linux file opening (`xdg-open`) |
| `install-linux.sh` | Linux installation script |
| `uninstall-linux.sh` | Linux uninstallation script |
| `nfpm.yaml` | .deb/.rpm package configuration |

---

## Verification

1. `GOOS=darwin go build ./cmd/service/` still compiles + `make test` passes on macOS
2. `GOOS=linux go build ./cmd/service/` compiles + `make test` passes on Linux
3. `go test ./cmd/service/` passes on both (common tests + platform-specific)
4. `make e2e` passes on both platforms
5. GUI compiles on Linux, file opening uses `xdg-open`
6. CLI unchanged — works identically on both
7. GitHub Actions: push → CI builds+tests on macOS+Linux; tag push → release with all artifacts
8. `.deb` installs on Ubuntu, `.rpm` on Fedora, service starts and indexes

---

## Decisions

| Decision | Rationale |
|----------|-----------|
| **fanotify over inotify** | `FAN_MARK_FILESYSTEM` monitors entire mounts with one FD, matching FSEvents design. Requires kernel 5.9+ and root. |
| **Separate install scripts** | Linux systemd patterns differ enough from launchd to warrant separate scripts. |
| **`nfpm` for packaging** | Generates both .deb and .rpm from single YAML config. No need for full dpkg/rpmbuild toolchains. |
| **Build tag approach** | Each `main_<os>.go` contains `main()`; Go toolchain selects at build time. No runtime OS detection in daemon. |
| **Minimum Linux kernel: 5.9+** | Required for `FAN_REPORT_DFID_NAME`. Covers Ubuntu 22.04+, Fedora 33+, Debian 11+. |

---

## Further Considerations

1. **`.desktop` file** — For Linux GUI launcher integration, create an `everythingx.desktop` file and include it in the packages.
2. **GUI build deps on Linux** — Fyne requires X11/Wayland dev libraries to build with CGO. The GitHub Actions workflow and LINUX.md should document: `apt-get install libgl1-mesa-dev xorg-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev`.
3. **Kernel version check** — The Linux daemon could validate kernel version at startup and print a clear error if < 5.9, rather than failing cryptically on fanotify init.
