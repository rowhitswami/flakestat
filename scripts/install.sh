#!/bin/sh
# Install flakestat from GitHub Releases.
#
#   curl -sSfL https://raw.githubusercontent.com/rowhitswami/flakestat/main/scripts/install.sh | sh
#   curl -sSfL .../install.sh | sh -s -- -b /usr/local/bin v0.1.0
#
# Flags:
#   -b <dir>   install directory (default ./bin, or $FLAKESTAT_INSTALL_DIR)
#   [version]  tag to install, e.g. v0.1.0 (default: latest)
#
# POSIX sh on purpose: this has to run in minimal CI images without bash.
set -eu

OWNER="rowhitswami"
REPO="flakestat"
BINARY="flakestat"

INSTALL_DIR="${FLAKESTAT_INSTALL_DIR:-./bin}"
VERSION=""

log()  { printf '%s\n' "$*" >&2; }
fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        -b) INSTALL_DIR="${2:-}"; [ -n "$INSTALL_DIR" ] || fail "-b needs a directory"; shift 2 ;;
        -h|--help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        -*) fail "unknown flag: $1" ;;
        *)  VERSION="$1"; shift ;;
    esac
done

# --- platform detection ------------------------------------------------------
# These must match the archive name_template in .goreleaser.yaml.

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
    linux)                 os="linux" ;;
    darwin)                os="darwin" ;;
    msys*|mingw*|cygwin*)  os="windows" ;;
    *) fail "unsupported OS: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
    x86_64|amd64)  arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) fail "unsupported architecture: $arch (flakestat ships amd64 and arm64)" ;;
esac

# --- download helpers --------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
    http_get() { curl -sSfL "$1" -o "$2"; }
    http_out() { curl -sSfL "$1"; }
elif command -v wget >/dev/null 2>&1; then
    http_get() { wget -qO "$2" "$1"; }
    http_out() { wget -qO- "$1"; }
else
    fail "need curl or wget"
fi

if [ -z "$VERSION" ]; then
    log "Resolving latest release..."
    # Follows the /latest redirect rather than hitting the API, which is rate
    # limited to 60/hour for unauthenticated CI runners.
    VERSION=$(http_out "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
        | tr ',' '\n' | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    [ -n "$VERSION" ] || fail "could not resolve the latest version; pass one explicitly, e.g. v0.1.0"
fi

# Version without the leading v, as used in archive names.
version_bare=$(printf '%s' "$VERSION" | sed 's/^v//')

ext="tar.gz"
[ "$os" = "windows" ] && ext="zip"

archive="${REPO}_${version_bare}_${os}_${arch}.${ext}"
base_url="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}"

tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT INT TERM

log "Downloading ${REPO} ${VERSION} (${os}/${arch})..."
http_get "${base_url}/${archive}" "${tmp}/${archive}" \
    || fail "download failed: ${base_url}/${archive}"

# --- checksum verification ---------------------------------------------------
# Fails closed. Every path that cannot produce a verified match aborts, because
# "warn and install anyway" means anyone who can drop the checksums request has
# silently downgraded this to no verification at all.
#
# FLAKESTAT_SKIP_CHECKSUM=1 opts out, for an image with no sha256 tool. It has
# to be set deliberately; it is never the default.

if [ "${FLAKESTAT_SKIP_CHECKSUM:-0}" = "1" ]; then
    log "warning: FLAKESTAT_SKIP_CHECKSUM=1, installing without verification"
else
    http_get "${base_url}/checksums.txt" "${tmp}/checksums.txt" 2>/dev/null \
        || fail "could not fetch checksums.txt from ${base_url}

  Refusing to install an unverified binary. Retry, or set
  FLAKESTAT_SKIP_CHECKSUM=1 if you accept the risk."

    expected=$(grep " ${archive}\$" "${tmp}/checksums.txt" | awk '{print $1}' || true)
    [ -n "$expected" ] || fail "no checksum listed for ${archive} in checksums.txt

  Refusing to install an unverified binary."

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "${tmp}/${archive}" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "${tmp}/${archive}" | awk '{print $1}')
    else
        fail "need sha256sum or shasum to verify the download

  Install one, or set FLAKESTAT_SKIP_CHECKSUM=1 if you accept the risk."
    fi

    [ "$actual" = "$expected" ] || fail "checksum mismatch for ${archive}
  expected: ${expected}
  actual:   ${actual}"
    log "Checksum verified."
fi

# --- extract and install -----------------------------------------------------

if [ "$ext" = "zip" ]; then
    command -v unzip >/dev/null 2>&1 || fail "need unzip to extract $archive"
    unzip -q "${tmp}/${archive}" -d "$tmp"
else
    tar -xzf "${tmp}/${archive}" -C "$tmp"
fi

bin_name="$BINARY"
[ "$os" = "windows" ] && bin_name="${BINARY}.exe"

[ -f "${tmp}/${bin_name}" ] || fail "archive did not contain ${bin_name}"

mkdir -p "$INSTALL_DIR"
install -m 0755 "${tmp}/${bin_name}" "${INSTALL_DIR}/${bin_name}" 2>/dev/null \
    || { cp "${tmp}/${bin_name}" "${INSTALL_DIR}/${bin_name}" && chmod 0755 "${INSTALL_DIR}/${bin_name}"; }

log "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${bin_name}"

case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *) log ""
       log "${INSTALL_DIR} is not on your PATH. Add it with:"
       log "  export PATH=\"\$PATH:$(cd "$INSTALL_DIR" 2>/dev/null && pwd || printf '%s' "$INSTALL_DIR")\"" ;;
esac
