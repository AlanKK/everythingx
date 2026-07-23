#!/bin/sh
# EverythingX universal installer.
#
#   curl -fsSL https://raw.githubusercontent.com/AlanKK/everythingx/main/install.sh | sh
#
# Detects your OS + CPU architecture, downloads the matching native package
# (.pkg on macOS, .deb/.rpm on Linux) from the latest GitHub Release, and
# installs it with the platform's native installer. Escalates to sudo only for
# the install step.
#
# Environment overrides:
#   EVERYTHINGX_VERSION   pin a specific release tag (e.g. v0.2.2-beta).
#                         Defaults to the latest release.
#
# POSIX sh only (no bashisms) — the pipe target is often dash.

set -eu

REPO="${EVERYTHINGX_REPO:-AlanKK/everythingx}"
VERSION="${EVERYTHINGX_VERSION:-latest}"

# ---- output helpers ---------------------------------------------------------
if [ -t 1 ]; then
    BOLD="$(printf '\033[1m')"; RED="$(printf '\033[31m')"
    GREEN="$(printf '\033[32m')"; RESET="$(printf '\033[0m')"
else
    BOLD=""; RED=""; GREEN=""; RESET=""
fi
info() { printf '%s\n' "${BOLD}==>${RESET} $*"; }
warn() { printf '%s\n' "${BOLD}${RED}warning:${RESET} $*" >&2; }
err()  { printf '%s\n' "${BOLD}${RED}error:${RESET} $*" >&2; exit 1; }

# ---- privilege escalation ---------------------------------------------------
# Under `curl | sh` there is no script file to re-exec, so prefix privileged
# commands with $SUDO rather than re-running ourselves.
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        SUDO="sudo"
    else
        err "This installer needs root. Re-run as root or install sudo."
    fi
fi

# ---- download helper --------------------------------------------------------
download() {
    _url="$1"; _out="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fL --progress-bar "$_url" -o "$_out" \
            || err "Download failed: $_url"
    elif command -v wget >/dev/null 2>&1; then
        # No --show-progress: busybox wget (minimal distros) rejects that flag.
        wget -O "$_out" "$_url" \
            || err "Download failed: $_url"
    else
        err "Need curl or wget to download packages."
    fi
}

# Resolve the download URL for an asset name against latest or a pinned tag.
asset_url() {
    if [ "$VERSION" = "latest" ]; then
        printf 'https://github.com/%s/releases/latest/download/%s' "$REPO" "$1"
    else
        printf 'https://github.com/%s/releases/download/%s/%s' "$REPO" "$VERSION" "$1"
    fi
}

# ---- detect architecture ----------------------------------------------------
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64 | amd64)          ARCH="amd64" ;;
    aarch64 | arm64)         ARCH="arm64" ;;
    *) err "Unsupported CPU architecture: $ARCH (only x86_64/amd64 and arm64/aarch64 are supported)." ;;
esac

OS="$(uname -s)"

# ---- temp workspace ---------------------------------------------------------
TMPDIR_EX="$(mktemp -d 2>/dev/null || mktemp -d -t everythingx)"
cleanup() { rm -rf "$TMPDIR_EX"; }
trap cleanup EXIT INT TERM

install_macos() {
    case "$ARCH" in
        arm64) asset="EverythingX_macos-apple-arm64.pkg" ;;
        amd64) asset="EverythingX_macos-intel-amd64.pkg" ;;
    esac
    out="$TMPDIR_EX/$asset"
    info "Downloading ${asset} ..."
    download "$(asset_url "$asset")" "$out"

    info "Installing (you may be prompted for your password) ..."
    $SUDO installer -pkg "$out" -target / \
        || err "installer failed. Try: sudo installer -pkg \"$out\" -target /"

    printf '\n%s\n' "${GREEN}${BOLD}EverythingX installed.${RESET}"
    cat <<'EOF'

Next step (required for full indexing):
  Grant Full Disk Access to everythingxd:
  System Settings -> Privacy & Security -> Full Disk Access -> add /usr/local/bin/everythingxd

Try it:
  ev -b <filename>        # search from the CLI
  open -a EverythingX     # launch the GUI
Logs: /var/log/everythingxd.log
EOF
}

install_linux() {
    # CGO/glibc binaries — musl (Alpine) is not supported.
    if command -v apk >/dev/null 2>&1 || grep -qs '^ID=alpine' /etc/os-release 2>/dev/null; then
        err "Alpine/musl is not supported. EverythingX ships glibc binaries (Debian, Ubuntu, Fedora, RHEL, openSUSE, ...)."
    fi

    # Choose package family by the tooling actually present on the host.
    if command -v dpkg >/dev/null 2>&1 || command -v apt-get >/dev/null 2>&1; then
        family="deb"
        asset="everythingx_linux_${ARCH}.deb"
    elif command -v rpm >/dev/null 2>&1 || command -v dnf >/dev/null 2>&1 \
        || command -v yum >/dev/null 2>&1 || command -v zypper >/dev/null 2>&1; then
        family="rpm"
        asset="everythingx_linux_${ARCH}.rpm"
    else
        err "No supported package manager found (need dpkg/apt for .deb or rpm/dnf/yum/zypper for .rpm)."
    fi

    out="$TMPDIR_EX/$asset"
    info "Downloading ${asset} ..."
    download "$(asset_url "$asset")" "$out"

    info "Installing (you may be prompted for your password) ..."
    if [ "$family" = "deb" ]; then
        if command -v apt-get >/dev/null 2>&1; then
            # dpkg first, then let apt resolve any missing dependencies.
            $SUDO dpkg -i "$out" || $SUDO apt-get install -f -y
        else
            $SUDO dpkg -i "$out" || err "dpkg install failed."
        fi
    else
        if command -v dnf >/dev/null 2>&1; then
            $SUDO dnf install -y "$out"
        elif command -v yum >/dev/null 2>&1; then
            $SUDO yum install -y "$out"
        elif command -v zypper >/dev/null 2>&1; then
            $SUDO zypper --non-interactive install --allow-unsigned-rpm "$out"
        else
            $SUDO rpm -i "$out" || err "rpm install failed."
        fi
    fi

    printf '\n%s\n' "${GREEN}${BOLD}EverythingX installed.${RESET}"
    if ! command -v systemctl >/dev/null 2>&1; then
        warn "systemd not detected — the daemon was not started automatically."
        printf '%s\n' "Start it manually with: sudo everythingxd"
    fi
    cat <<'EOF'

The everythingxd daemon needs root for fanotify (Linux kernel 5.9+).
Try it:
  ev -b <filename>                  # search from the CLI
  everythingx                       # launch the GUI
  systemctl status everythingxd     # check the service
Logs: /var/log/everythingxd.log
EOF
}

case "$OS" in
    Darwin) install_macos ;;
    Linux)  install_linux ;;
    *)      err "Unsupported operating system: $OS (supported: macOS, Linux)." ;;
esac
