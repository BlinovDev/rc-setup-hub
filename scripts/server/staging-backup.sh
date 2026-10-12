#!/usr/bin/env bash
# Root-only timer entry point; shares the staging deployment lock.
set -euo pipefail
umask 077
exec /usr/bin/python3 /usr/local/lib/rc-hub/staging_media.py
