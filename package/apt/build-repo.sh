#!/bin/sh
# Build a signed apt repository from a set of .deb files.
#   build-repo.sh <out-dir> <deb>...
# Keeps the newest $KEEP versions per arch. Signs with the default secret key
# in the gpg keyring, or $APT_GPG_KEY_ID if set; $APT_GPG_PASSPHRASE is used
# if the key has one. Needs dpkg-deb, apt-ftparchive and gpg.
set -eu

KEEP=${KEEP:-5}
ARCHES="amd64 arm64"

if [ $# -lt 2 ]; then
    echo "usage: $0 <out-dir> <deb>..." >&2
    exit 2
fi
out=$1
shift

rm -rf "$out"
mkdir -p "$out/pool/main/e/everythingx"
list=$(mktemp)
trap 'rm -f "$list"' EXIT

# One "arch version path" line per deb. Read the version from the package
# itself: nfpm writes -beta as ~beta, and the two sort differently.
for deb in "$@"; do
    fields=$(dpkg-deb -f "$deb" Version Architecture)
    ver=$(printf '%s\n' "$fields" | sed -n 's/^Version: //p')
    arch=$(printf '%s\n' "$fields" | sed -n 's/^Architecture: //p')
    printf '%s %s %s\n' "$arch" "$ver" "$deb" >> "$list"
done

for arch in $ARCHES; do
    # Pick the highest remaining version $KEEP times; duplicates of a version
    # already picked (the same deb in two releases) are dropped.
    kept=""
    n=0
    while [ "$n" -lt "$KEEP" ]; do
        best="" best_path=""
        while read -r a v p; do
            [ "$a" = "$arch" ] || continue
            case " $kept " in *" $v "*) continue ;; esac
            if [ -z "$best" ] || dpkg --compare-versions "$v" gt "$best"; then
                best=$v best_path=$p
            fi
        done < "$list"
        [ -n "$best" ] || break
        echo "keep $arch $best"
        # Filename: in Packages is free-form; keep '~' out of URLs.
        cp "$best_path" "$out/pool/main/e/everythingx/everythingx_$(printf '%s' "$best" | tr '~' '-')_${arch}.deb"
        kept="$kept $best"
        n=$((n + 1))
    done
done

# Filename: paths in Packages are relative to the repo root, so run from there.
for arch in $ARCHES; do
    dir="dists/stable/main/binary-$arch"
    mkdir -p "$out/$dir"
    (cd "$out" && apt-ftparchive --arch "$arch" packages pool/ > "$dir/Packages")
    gzip -9 -k "$out/$dir/Packages"
done

# Write Release to a temp file first so it doesn't list (and hash) itself.
(
    cd "$out/dists/stable"
    apt-ftparchive \
        -o APT::FTPArchive::Release::Origin=EverythingX \
        -o APT::FTPArchive::Release::Label=EverythingX \
        -o APT::FTPArchive::Release::Suite=stable \
        -o APT::FTPArchive::Release::Codename=stable \
        -o "APT::FTPArchive::Release::Architectures=$ARCHES" \
        -o APT::FTPArchive::Release::Components=main \
        release . > ../Release.tmp
    mv ../Release.tmp Release
)

# Pin one key for signing and export, so a local keyring with several keys
# doesn't put all of them in everythingx.gpg.
key=${APT_GPG_KEY_ID:-$(gpg --batch --list-secret-keys --with-colons | awk -F: '$1 == "fpr" { print $10; exit }')}
[ -n "$key" ] || { echo "no secret key in the gpg keyring" >&2; exit 1; }
set -- --batch --yes --pinentry-mode loopback --local-user "$key"
if [ -n "${APT_GPG_PASSPHRASE:-}" ]; then
    set -- "$@" --passphrase-fd 0
fi
sign() {
    printf '%s\n' "${APT_GPG_PASSPHRASE:-}" | gpg "$@"
}
rel="$out/dists/stable/Release"
sign "$@" --clearsign --output "$out/dists/stable/InRelease" "$rel"
sign "$@" --armor --detach-sign --output "$out/dists/stable/Release.gpg" "$rel"

# Public key, binary (dearmored) as /etc/apt/keyrings expects.
gpg --batch --export "$key" > "$out/everythingx.gpg"
[ -s "$out/everythingx.gpg" ] || { echo "no public key exported" >&2; exit 1; }

cat > "$out/index.html" <<'EOF'
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EverythingX apt repository</title></head>
<body>
<h1>EverythingX apt repository</h1>
<p>Debian/Ubuntu packages for <a href="https://github.com/AlanKK/everythingx">EverythingX</a> (amd64, arm64).</p>
<pre>
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://alankk.github.io/everythingx/everythingx.gpg | sudo tee /etc/apt/keyrings/everythingx.gpg >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/everythingx.gpg] https://alankk.github.io/everythingx stable main" | sudo tee /etc/apt/sources.list.d/everythingx.list
sudo apt update &amp;&amp; sudo apt install everythingx
</pre>
</body>
</html>
EOF
