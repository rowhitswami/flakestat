"""flakestat - find flaky tests in any language.

This package is a thin wrapper that fetches and runs the flakestat binary.
The tool itself is written in Go; see https://github.com/rowhitswami/flakestat.
"""

import subprocess
import sys

from ._binary import InstallError, ensure_binary

# Read from installed package metadata so pyproject.toml is the single source
# of truth. A hardcoded constant here silently drifted from the version CI
# stamped at publish time, and since the download URL is built from it, the
# published package asked GitHub for a release tag that did not exist.
try:
    from importlib.metadata import version as _pkg_version

    __version__ = _pkg_version("flakestat")
except Exception:  # not installed, e.g. running from a source checkout
    __version__ = "0.0.0"

__all__ = ["main", "ensure_binary", "InstallError", "__version__"]


def main() -> int:
    """Console-script entry point: exec the binary, preserving the exit code."""
    try:
        binary = ensure_binary()
    except InstallError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    try:
        # CI gates depend on the exit code, so pass it straight through.
        return subprocess.call([str(binary)] + sys.argv[1:])
    except KeyboardInterrupt:
        return 130
    except OSError as exc:
        print("flakestat: {}".format(exc), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
