# Release Process

## How CI Works

| Trigger | What runs |
|---|---|
| Push to `main` | Build + unit tests + macOS e2e tests. No package builds. |
| Manual dispatch | Full pipeline: build, package builds, install tests on all platforms. |
| Tag `v*` | Same as manual dispatch, then creates a GitHub Release if all tests pass. |

Package install tests (`.pkg`, `.deb`, `.rpm`) only run on manual dispatch and tags — not on every push to `main`.

## Creating a Release

1. Push your changes to `main` and wait for CI to go green.

2. Run the full pipeline manually to verify packages install correctly on all platforms:
   ```bash
   gh workflow run main.yml
   ```
   Wait for all jobs to pass: build × 3 + package tests × 3 (macOS pkg × 2, Linux deb, Linux rpm).

3. Tag the release:
   ```bash
   git tag v1.0-beta-1
   git push origin v1.0-beta-1
   ```

4. CI runs the full pipeline on the tag. If all jobs pass, a GitHub Release is created automatically with release notes and all packages.

## Version Scheme

- Tags must start with `v` followed by a digit (e.g., `v1.0.0`, `v1.0-beta-1`, `v2.1.0-rc.1`)
- The `v` prefix is stripped for package versions (e.g., `v1.0-beta-1` → `1.0-beta-1`)
- Non-tag builds use `0.0.0-dev` as the package version
- Version, commit, and build date are embedded in all binaries via `-ldflags`
- Check with: `ev --version` or `everythingxd --version`

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
gh workflow run "Test Published Release" -f tag=v1.0-beta-1
```

This downloads the release assets from GitHub and tests installation on each platform.
