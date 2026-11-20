#!/usr/bin/env bash
# Cut a release: check that main is clean, in sync and green in CI, then push an
# annotated version tag. The tag push triggers the release jobs in main.yml.
#
#   package/release.sh v1.1.0         # stable: GitHub Latest, Homebrew, apt
#   package/release.sh v1.2.0-rc.1    # pre-release: GitHub pre-release only
#   make release TAG=v1.1.0
set -euo pipefail

die() { echo "error: $*" >&2; exit 1; }

tag="${1:-}"
[[ -n "$tag" ]] || die "usage: $0 vMAJOR.MINOR.PATCH[-pre]"
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] \
  || die "'$tag' is not vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-pre (e.g. v1.1.0, v1.2.0-rc.1)"

command -v gh >/dev/null || die "gh CLI is required to check CI status"

branch=$(git rev-parse --abbrev-ref HEAD)
[[ "$branch" == "main" ]] || die "on branch '$branch'; switch to main first"
[[ -z "$(git status --porcelain)" ]] || die "working tree is not clean"

git fetch --quiet --tags origin main
head=$(git rev-parse HEAD)
[[ "$head" == "$(git rev-parse origin/main)" ]] \
  || die "local main ($(git rev-parse --short HEAD)) differs from origin/main ($(git rev-parse --short origin/main)); pull or push first"

git rev-parse -q --verify "refs/tags/$tag" >/dev/null && die "tag $tag already exists"

# A stable tag must be the highest stable version; a hotfix on an older line
# would be skipped by the brew/apt version guards anyway, so stop early.
if [[ "$tag" != *-* ]]; then
  latest=$(git tag -l 'v*.*.*' | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -1 || true)
  if [[ -n "$latest" && "$(printf '%s\n%s\n' "$latest" "$tag" | sort -V | tail -1)" != "$tag" ]]; then
    die "$tag is not newer than the latest stable tag $latest"
  fi
fi

# CI must have passed on exactly this commit.
read -r run_sha conclusion url < <(gh run list --workflow main.yml --branch main --event push --limit 1 \
  --json headSha,conclusion,url --jq '.[0] | "\(.headSha) \(.conclusion) \(.url)"')
[[ "$run_sha" == "$head" ]] \
  || die "latest main CI run is for ${run_sha:0:7}, not HEAD ${head:0:7}; wait for CI on HEAD"
[[ "$conclusion" == "success" ]] \
  || die "CI on ${head:0:7} is '${conclusion:-in progress}': $url"

echo "Commit:  $(git log -1 --format='%h %s' HEAD)"
echo "CI:      success ($url)"
echo "Tag:     $tag$([[ "$tag" == *-* ]] && echo ' (pre-release)')"
read -r -p "Create and push $tag? [y/N] " reply
[[ "$reply" == [yY] ]] || die "aborted"

git tag -a "$tag" -m "$tag"
git push origin "refs/tags/$tag"
echo "Pushed $tag. Watch the release: gh run watch \$(gh run list --workflow main.yml --branch $tag --limit 1 --json databaseId --jq '.[0].databaseId')"
