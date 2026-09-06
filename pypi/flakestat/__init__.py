"""flakestat - find flaky tests in any language.

This package is a thin wrapper that fetches and runs the flakestat binary.
The tool itself is written in Go; see https://github.com/rowhitswami/flakestat.
"""

import subprocess
import sys

from ._binary import InstallError, ensure_binary

# Kept in step with the released binary by CI at publish time.
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
