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

Auto builds bump the patch of the highest `v1.x.y` tag: `v1.0.0`, `v1.0.1`, `v1.0.2`, and so on (the first
release on a repo with no `v1.*.*` tag is `v1.0.0`).

To move the minor or major version, tag it by hand; later auto builds continue from that tag:

```bash
git tag v1.1.0
git push origin v1.1.0
```

Manual tags must be `v` followed by a digit (`v1.1.0`, `v2.1.0-rc.1`). The auto-tag uses
`GITHUB_TOKEN`, so it doesn't re-trigger the workflow.

Package versions drop the `v`. deb/rpm need a leading digit, and nfpm writes `-beta` as
`~beta` for pre-release tags (`everythingx_0.2.1~beta_amd64.deb`). Version, commit and build date are baked in
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

## apt repository

After each release, the `publish-apt` job rebuilds the apt repo at
[alankk.github.io/everythingx](https://alankk.github.io/everythingx) and deploys it to GitHub
Pages. It downloads the `.deb` assets of the 20 newest releases and runs
`package/apt/build-repo.sh site dl/*/*.deb`, which:

- reads each deb's version and arch from the package (`0.2.9~beta`, not the tag's `0.2.9-beta`)
- keeps the 5 highest versions per arch (`dpkg --compare-versions`) in `pool/main/e/everythingx/`
- writes `dists/stable/main/binary-{amd64,arm64}/Packages{,.gz}` and `dists/stable/Release` with
  `apt-ftparchive`, and signs them as `InRelease` and `Release.gpg`
- exports the public key to `everythingx.gpg` and writes a small `index.html`

Nothing is committed to git. Every run rebuilds the whole repo from Releases, so a run skipped by
the `publish-apt` concurrency group loses nothing. Because the highest versions win, a
`0.2.N-beta` built after a manual `v1.0.0` never becomes the candidate, and `apt upgrade` never
downgrades.

To build it locally (needs `apt-utils` and a secret key in your gpg keyring; set
`APT_GPG_KEY_ID` to pick one):

```bash
package/apt/build-repo.sh site path/to/*.deb
cd site && python3 -m http.server
```

**One-time setup.**
1. Generate a signing key and store it as Actions secrets:
   ```bash
   gpg --quick-gen-key "EverythingX apt <AlanKK@users.noreply.github.com>" rsa4096 sign never
   gpg --armor --export-secret-keys <key-id>   # -> secret APT_GPG_PRIVATE_KEY
   ```
   If the key has a passphrase, store it as `APT_GPG_PASSPHRASE`. Keep a backup of the key.
   Users trust it by fingerprint, so a new key means everyone has to re-download `everythingx.gpg`.
2. Settings → Pages → Source: **GitHub Actions**.
3. Settings → Environments → `github-pages` → Deployment branches and tags: allow `main` and tags
   matching `v*`; otherwise runs triggered by a manual tag can't deploy.

## Smoke test

To re-test a published release on clean runners:

```bash
gh workflow run "Test Published Release" -f tag=v1.0-beta-1
```

`test-brew` and `test-apt` install from the tap and the apt repo. They fail if those serve a
different version from the tag under test, which is expected when the tag isn't the highest
version (the brew version guard skipped it, or apt prefers a higher one).
