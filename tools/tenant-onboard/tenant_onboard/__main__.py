"""Allow ``python -m tenant_onboard``."""

import sys

from .cli import main

sys.exit(main())
