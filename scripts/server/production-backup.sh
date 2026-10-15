#!/usr/bin/env bash
# Root-only timer entry point; shares the production deployment lock.
set -euo pipefail
umask 077
exec /usr/bin/python3 /usr/local/lib/rc-hub/production_media.py
