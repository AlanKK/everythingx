# Releases

Releases are cut by pushing a version tag. Merges to `main` only build and test.

| Trigger | What runs |
|---|---|
| PR to `main`, push to `main`, manual dispatch on a branch | Build, unit tests, e2e and packaging on all platforms (`0.0.0-dev`), no release |
| Tag `vX.Y.Z` | Build and test with that version, publish a GitHub release as Latest, update the Homebrew tap and the apt repo |
| Tag `vX.Y.Z-pre` (e.g. `v1.2.0-rc.1`) | Build and test, publish a GitHub **pre-release** only: not Latest, not pushed to Homebrew or apt |

The tag run fails before building if the tag isn't `vMAJOR.MINOR.PATCH[-pre]` or if the tagged
commit isn't on `main`. Superseded PR runs are cancelled; `main` and tag runs always finish.

## Cutting a release

Batch up as many merges as you like, then:

```bash
git switch main && git pull
make release TAG=v1.1.0        # or: package/release.sh v1.1.0
```

`package/release.sh` refuses to tag unless you're on `main` with a clean tree that matches
`origin/main`, the latest `main.yml` run on that commit succeeded, the tag doesn't exist, and (for a
stable tag) it is higher than the newest stable tag. It shows the commit and CI run, asks for
confirmation, then creates an annotated tag and pushes only that tag. Doing it by hand is the same as:

```bash
git tag -a v1.1.0 -m "v1.1.0"
git push origin v1.1.0
```

If a tag run fails, fix it on `main` and tag the next patch version. To retry a run that failed
for a transient reason, re-run it from the Actions tab.

## Versions

Package versions are the tag without the `v`. nfpm writes `-rc.1` as `~rc.1` in deb/rpm
(`everythingx_1.2.0~rc.1_amd64.deb`), so a pre-release sorts below its final version. Version,
commit and build date are baked in with `-ldflags`; check with `ev --version` or
`everythingxd --version`. Non-release builds report `0.0.0-dev`.

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
`releases/latest/download/`. Each Linux arch is built natively (`ubuntu-24.04` for amd64,
`ubuntu-24.04-arm` for arm64) and the `release` job only collects the artifacts.

## Homebrew tap

After each stable release, the `publish-homebrew` job renders `package/homebrew/everythingx.rb.tmpl`
with the release's tag, version and `.pkg` checksums (`package/homebrew/render-cask.sh`), and
pushes it to `Casks/everythingx.rb` in
[`AlanKK/homebrew-everythingx`](https://github.com/AlanKK/homebrew-everythingx). Users install
with `brew install --cask alankk/everythingx/everythingx`.

**Version guard.** A stable tag that is lower than what the tap serves (say a `v1.0.5` hotfix
tagged after `v1.1.0`) would make `brew upgrade` downgrade users. The job reads the cask's current
`version` and compares it with `dpkg --compare-versions`, with `-` mapped to `~` as nfpm does. If
the new version isn't higher, it logs "skipping" and pushes nothing. The job runs in the `publish-brew` concurrency
group, so two releases never push at once.

**One-time setup.**
1. Create the public repo `AlanKK/homebrew-everythingx` with at least one commit (a README is
   enough; `actions/checkout` can't check out an empty repo). The job creates `Casks/`.
2. Create a fine-grained PAT with **Contents: Read and write** on that repo only.
3. Store it in this repo as the Actions secret `HOMEBREW_TAP_TOKEN`.

The `.pkg` is not signed or notarized. The `test-brew` job in the smoke test checks that
Homebrew still installs it.

## apt repository

After each stable release, the `publish-apt` job rebuilds the apt repo at
[alankk.github.io/everythingx](https://alankk.github.io/everythingx) and deploys it to GitHub
Pages. It downloads the `.deb` assets of the 20 newest non-pre-releases and runs
`package/apt/build-repo.sh site dl/*/*.deb`, which:

- reads each deb's version and arch from the package (`1.2.0~rc.1`, not the tag's `1.2.0-rc.1`)
- keeps the 5 highest versions per arch (`dpkg --compare-versions`) in `pool/main/e/everythingx/`
- writes `dists/stable/main/binary-{amd64,arm64}/Packages{,.gz}` and `dists/stable/Release` with
  `apt-ftparchive`, and signs them as `InRelease` and `Release.gpg`
- exports the public key to `everythingx.gpg` and writes a small `index.html`

Nothing is committed to git. Every run rebuilds the whole repo from Releases, so a run skipped by
the `publish-apt` concurrency group loses nothing. Because the highest versions win, an older
hotfix tag never becomes the candidate, and `apt upgrade` never downgrades.

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
gh workflow run "Test Published Release" -f tag=v1.1.0
```

`test-brew` and `test-apt` install from the tap and the apt repo. They fail if those serve a
different version from the tag under test, which is expected when the tag isn't the highest
version (the brew version guard skipped it, apt prefers a higher one, or it's a pre-release).
