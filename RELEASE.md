# Release Process

## How CI Works

Every push to `main` triggers the full pipeline:

1. **Build** — compiles binaries, runs unit tests and macOS e2e tests on all 3 platforms (macOS arm64, macOS amd64, Linux amd64). Builds `.pkg`, `.deb`, and `.rpm` packages.
2. **Package tests** — installs the built packages on clean runners and verifies they work:
   - macOS `.pkg` installed via `installer`, app bundle verified
   - Linux `.deb` installed via `dpkg`, systemd service verified, e2e test run
   - Linux `.rpm` extracted and installed, e2e test run
3. **Release** — only runs on `v*` tags, after all test jobs pass. Creates a GitHub Release with `.zip`, `.pkg`, `.deb`, and `.rpm` artifacts.

## Creating a Release

1. Push your changes to `main` and wait for CI to go green (all 5 jobs must pass: build × 3 + package tests × 3).

2. Tag the release:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

3. CI runs the full pipeline again on the tag. If all jobs pass, a GitHub Release is created automatically with release notes and all packages.

## Artifacts Produced

| Artifact | Platform | Contents |
|---|---|---|
| `everythingx_macos-apple-arm64.zip` | macOS Apple Silicon | `everythingxd`, `ev`, `everythingx`, `EverythingX.app`, install/uninstall scripts, launchd plist |
| `everythingx_macos-intel-amd64.zip` | macOS Intel | Same as above |
| `EverythingX_macos-apple-arm64.pkg` | macOS Apple Silicon | Installer package (installs `EverythingX.app` to `/Applications`) |
| `EverythingX_macos-intel-amd64.pkg` | macOS Intel | Same as above |
| `everythingx_<version>_amd64.deb` | Linux (Debian/Ubuntu) | `everythingxd`, `ev`, systemd service, desktop entry |
| `everythingx-<version>-1.x86_64.rpm` | Linux (Fedora/RHEL) | Same as above |

## Manual Smoke Test

After publishing a release, you can re-test the published artifacts on clean runners:

```bash
gh workflow run "Test Published Release" -f tag=v1.0.0
```

This downloads the release assets from GitHub and tests installation on each platform.

## Version Scheme

- Tags must start with `v` (e.g., `v1.0.0`, `v0.2.0-beta.1`)
- Non-tag builds use `0.0.0-dev` as the package version
- Version, commit, and build date are embedded in all binaries via `-ldflags`
- Check with: `ev --version` or `everythingxd --version`
