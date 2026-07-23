# Release Process

## How CI Works

| Trigger | What runs |
|---|---|
| Push/merge to `main` | **Auto-release.** Build + tests on all platforms, then auto-increments the build counter and publishes a "Latest" GitHub Release with the macOS `.pkg` installers + Linux `.deb`/`.rpm`. |
| Tag `v*` | Build + tests, then a GitHub Release using the tag's version. |
| PR / other branch / manual dispatch | Build + unit tests only (`0.0.0-dev`). No release. |

Runs are serialized per-ref (`concurrency`), so two merges to `main` in quick succession queue rather than race — each gets its own incremented version.

## Releasing

**You don't have to do anything.** Every merge to `main` automatically:

1. Reads the highest existing `v0.0.N` tag and computes the next one (`v0.0.N+1`).
2. Builds and tests on every platform.
3. If everything passes, creates that tag at the merge commit and publishes it as the **Latest** GitHub Release with auto-generated notes and all packages attached.

If any build or test fails, no tag or release is created — fix and merge again.

### Manual (semver) releases

To cut a versioned release outside the build counter (e.g. a real `v1.0.0`), tag it yourself:

```bash
git tag v1.0.0
git push origin v1.0.0
```

CI runs on the tag and publishes the release. Tag-created events use `GITHUB_TOKEN`, so the auto-tag from a `main` build never re-triggers the workflow.

## Version Scheme

- **Auto builds** (merge to `main`) use a monotonic build counter: `v0.0.1`, `v0.0.2`, … derived from the latest `v0.0.N` tag.
- **Manual tags** must start with `v` followed by a digit (e.g., `v1.0.0`, `v1.0-beta-1`, `v2.1.0-rc.1`). Bump major/minor by tagging manually; the counter resumes from the next `v0.0.N`.
- The `v` prefix is stripped for package versions (e.g., `v0.0.42` → `0.0.42`). deb/rpm require a version starting with a digit.
- PR / feature-branch / manual-dispatch builds use `0.0.0-dev`.
- Version, commit, and build date are embedded in all binaries via `-ldflags`.
- Check with: `ev --version` or `everythingxd --version`

## Artifacts Produced

| Artifact | Platform | Contents |
|---|---|---|
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
