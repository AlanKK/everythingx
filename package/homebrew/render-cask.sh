#!/bin/sh
# Render the Homebrew cask for a release and write it to stdout.
#   render-cask.sh <tag> <version> <arm.pkg> <intel.pkg> > Casks/everythingx.rb
# The tag goes into the download URL; the version is what brew compares.
# They differ for tags that don't start with a digit (see main.yml).
set -eu

if [ $# -ne 4 ]; then
    echo "usage: $0 <tag> <version> <arm.pkg> <intel.pkg>" >&2
    exit 2
fi
tag=$1 version=$2 arm_pkg=$3 intel_pkg=$4

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

sha_arm=$(sha256 "$arm_pkg")
sha_intel=$(sha256 "$intel_pkg")
tmpl="$(dirname "$0")/everythingx.rb.tmpl"

sed -e "s|@TAG@|${tag}|g" \
    -e "s|@VERSION@|${version}|g" \
    -e "s|@SHA_ARM@|${sha_arm}|g" \
    -e "s|@SHA_INTEL@|${sha_intel}|g" \
    "$tmpl"
