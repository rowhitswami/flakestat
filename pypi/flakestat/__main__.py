"""Support `python -m flakestat` alongside the console script."""

import sys

from . import main

if __name__ == "__main__":
    sys.exit(main())
