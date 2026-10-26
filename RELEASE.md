# Releases

Every merge to `main` builds and tests on all platforms, then tags and publishes a new
release automatically. If a build or test fails, nothing gets tagged; fix it and merge again.

| Trigger | What runs |
|---|---|
| Merge to `main` | Build, test, bump the build counter, publish as Latest with the `.pkg`, `.deb` and `.rpm` files |
| Tag `v*` | Build, test, publish a release with the tag's version |
| PR, other branches, manual dispatch | Build and unit tests only (`0.0.0-dev`), no release |

Runs are serialized per ref, so two quick merges queue up and each gets its own version.

## Versions

Auto builds count up from the highest `v0.2.N-beta` tag: `v0.2.1-beta`, `v0.2.2-beta`, and so on.

For a real version, tag it by hand:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Manual tags must be `v` followed by a digit (`v1.0.0`, `v2.1.0-rc.1`). The auto-tag uses
`GITHUB_TOKEN`, so it doesn't re-trigger the workflow.

Package versions drop the `v`. deb/rpm need a leading digit, and nfpm writes `-beta` as
`~beta` (`everythingx_0.2.1~beta_amd64.deb`). Version, commit and build date are baked in
with `-ldflags`; check with `ev --version` or `everythingxd --version`.

## Artifacts

| Artifact | Platform | Contents |
|---|---|---|
| `EverythingX_macos-apple-arm64.pkg` | macOS Apple Silicon | `EverythingX.app` → `/Applications`, `ev`/`everythingxd` → `/usr/local/bin`, launchd plist → `/Library/LaunchDaemons`; postinstall starts the daemon |
| `EverythingX_macos-intel-amd64.pkg` | macOS Intel | Same as above |
| `everythingx_linux_amd64.deb` | Linux x86_64 (Debian/Ubuntu) | `everythingxd`, `ev`, systemd service, desktop entry |
| `everythingx_linux_arm64.deb` | Linux arm64 (Debian/Ubuntu) | Same as above |
| `everythingx_linux_amd64.rpm` | Linux x86_64 (Fedora/RHEL) | Same as above |
| `everythingx_linux_arm64.rpm` | Linux arm64 (Fedora/RHEL) | Same as above |

Asset names don't include the version, so `install.sh` can fetch them from
`releases/latest/download/`. Each Linux arch is built natively (`ubuntu-latest` for amd64,
`ubuntu-24.04-arm` for arm64) and the `release` job only collects the artifacts.

## Homebrew tap

After each release, the `publish-homebrew` job renders `package/homebrew/everythingx.rb.tmpl`
with the release's tag, version and `.pkg` checksums (`package/homebrew/render-cask.sh`), and
pushes it to `Casks/everythingx.rb` in
[`AlanKK/homebrew-everythingx`](https://github.com/AlanKK/homebrew-everythingx). Users install
with `brew install --cask alankk/everythingx/everythingx`.

**Version guard.** Every release is published, so a `0.2.N-beta` built after a manual `v1.0.0`
would sort lower, and `brew upgrade` would treat it as an update and downgrade users. The job
reads the cask's current `version` and compares it with `dpkg --compare-versions`, with `-`
mapped to `~` as nfpm does (`0.2.9~beta` < `0.2.10~beta` < `1.0.0`). If the new version isn't
higher, it logs "skipping" and pushes nothing. The job runs in the `publish-brew` concurrency
group, so two releases never push at once.

**One-time setup.**
1. Create the public repo `AlanKK/homebrew-everythingx` with at least one commit (a README is
   enough; `actions/checkout` can't check out an empty repo). The job creates `Casks/`.
2. Create a fine-grained PAT with **Contents: Read and write** on that repo only.
3. Store it in this repo as the Actions secret `HOMEBREW_TAP_TOKEN`.

The `.pkg` is not signed or notarized. The `test-brew` job in the smoke test checks that
Homebrew still installs it.

## Smoke test

To re-test a published release on clean runners:

```bash
gh workflow run "Test Published Release" -f tag=v1.0-beta-1
```

`test-brew` installs from the tap and fails if the tap serves a different version from the
tag under test, which is expected when the version guard skipped that tag.
