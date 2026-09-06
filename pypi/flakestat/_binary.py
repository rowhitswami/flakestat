"""Resolve the flakestat binary, downloading it on first use.

pip has no post-install hook, and building a Go binary at install time would
require a Go toolchain on the user's machine. So the binary is fetched from
GitHub Releases the first time it is needed and cached per version.

Everything here is stdlib: the wrapper must never interfere with a user's
dependency resolution.
"""

import hashlib
import os
import platform
import shutil
import stat
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request
import zipfile
from pathlib import Path

OWNER = "rowhitswami"
REPO = "flakestat"


class InstallError(RuntimeError):
    """Raised when the binary cannot be obtained."""


def _version() -> str:
    from . import __version__

    return __version__


def _target():
    """Map this interpreter's platform onto the release archive naming."""
    system = platform.system().lower()
    goos = {"darwin": "darwin", "linux": "linux", "windows": "windows"}.get(system)

    machine = platform.machine().lower()
    goarch = {
        "x86_64": "amd64",
        "amd64": "amd64",
        "aarch64": "arm64",
        "arm64": "arm64",
    }.get(machine)

    if not goos or not goarch:
        raise InstallError(
            "flakestat: unsupported platform {}/{}.\n"
            "Prebuilt binaries cover macOS, Linux and Windows on x86_64 and arm64.\n"
            "Build from source instead:\n"
            "  go install github.com/{}/{}/cmd/{}@latest".format(
                system, machine, OWNER, REPO, REPO
            )
        )

    return goos, goarch


def _cache_dir() -> Path:
    """Per-version cache, so upgrading the wrapper fetches a matching binary."""
    if os.environ.get("FLAKESTAT_CACHE_DIR"):
        base = Path(os.environ["FLAKESTAT_CACHE_DIR"])
    elif sys.platform == "win32":
        base = Path(os.environ.get("LOCALAPPDATA", Path.home() / "AppData" / "Local"))
    elif sys.platform == "darwin":
        base = Path.home() / "Library" / "Caches"
    else:
        base = Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache"))

    return base / "flakestat" / _version()


def _binary_name() -> str:
    return "flakestat.exe" if sys.platform == "win32" else "flakestat"


def _fetch(url: str) -> bytes:
    try:
        with urllib.request.urlopen(url) as resp:  # noqa: S310 - fixed https host
            return resp.read()
    except urllib.error.HTTPError as exc:
        raise InstallError("flakestat: GET {} -> {}".format(url, exc.code)) from exc
    except urllib.error.URLError as exc:
        raise InstallError(
            "flakestat: could not reach {} ({})".format(url, exc.reason)
        ) from exc


def _verify(archive_name: str, blob: bytes, base_url: str) -> None:
    try:
        sums = _fetch("{}/checksums.txt".format(base_url)).decode("utf-8")
    except InstallError:
        print("flakestat: checksums.txt unavailable, skipping verification", file=sys.stderr)
        return

    expected = None
    for line in sums.splitlines():
        if line.strip().endswith(archive_name):
            expected = line.split()[0]
            break

    if expected is None:
        return

    actual = hashlib.sha256(blob).hexdigest()
    if expected != actual:
        raise InstallError(
            "flakestat: checksum mismatch for {}\n  expected: {}\n  actual:   {}".format(
                archive_name, expected, actual
            )
        )


def _download(dest: Path) -> None:
    goos, goarch = _target()
    version = _version()
    ext = "zip" if goos == "windows" else "tar.gz"

    archive = "{}_{}_{}_{}.{}".format(REPO, version, goos, goarch, ext)
    base_url = "https://github.com/{}/{}/releases/download/v{}".format(OWNER, REPO, version)

    print(
        "flakestat: downloading {} v{} ({}/{})".format(REPO, version, goos, goarch),
        file=sys.stderr,
    )

    blob = _fetch("{}/{}".format(base_url, archive))
    _verify(archive, blob, base_url)

    name = _binary_name()

    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        archive_path = tmp_path / archive
        archive_path.write_bytes(blob)

        if ext == "zip":
            with zipfile.ZipFile(archive_path) as zf:
                zf.extract(name, tmp_path)
        else:
            with tarfile.open(archive_path) as tf:
                member = tf.getmember(name)
                # Refuse absolute paths and traversal in archive members.
                if member.name != name or Path(member.name).is_absolute():
                    raise InstallError("flakestat: unexpected archive member {}".format(member.name))
                tf.extract(member, tmp_path)

        dest.parent.mkdir(parents=True, exist_ok=True)

        # Move into place via a temporary name so concurrent installs cannot
        # observe a half-written binary.
        staged = dest.parent / (dest.name + ".tmp")
        shutil.move(str(tmp_path / name), str(staged))
        staged.chmod(staged.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
        os.replace(str(staged), str(dest))

    print("flakestat: installed to {}".format(dest), file=sys.stderr)


def ensure_binary() -> Path:
    """Return the path to the binary, downloading it if needed."""
    override = os.environ.get("FLAKESTAT_BINARY")
    if override:
        path = Path(override)
        if not path.exists():
            raise InstallError("flakestat: FLAKESTAT_BINARY={} does not exist".format(override))
        return path

    dest = _cache_dir() / _binary_name()
    if dest.exists():
        return dest

    if os.environ.get("FLAKESTAT_SKIP_DOWNLOAD"):
        raise InstallError(
            "flakestat: binary missing and FLAKESTAT_SKIP_DOWNLOAD is set"
        )

    _download(dest)
    return dest
