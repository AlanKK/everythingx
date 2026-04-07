#!/bin/bash

set -euo pipefail

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <tag>"
    exit 1
fi

TAG="$1"

if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: this script must be run inside a git repository."
    exit 1
fi

if git rev-parse "$TAG" > /dev/null 2>&1; then
    echo "Deleting local tag: $TAG"
    git tag -d "$TAG"
else
    echo "Local tag not found: $TAG"
fi

echo "Deleting remote tag: $TAG"
git push origin ":refs/tags/$TAG"

echo "Recreating tag at current HEAD: $TAG"
git tag -a "$TAG" -m "$TAG"

echo "Pushing tag: $TAG"
git push origin "$TAG"

echo "Done. Tag replaced: $TAG"
