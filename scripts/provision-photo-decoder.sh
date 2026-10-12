#!/usr/bin/env bash
# Run only on an authorized host/CI runner. Does not touch databases or media.
set -euo pipefail
source /etc/os-release
if [[ "$ID" != ubuntu || "$VERSION_ID" != 24.04 ]]; then
  echo 'Photo decoder provisioning currently supports Ubuntu 24.04 (amd64/arm64) only' >&2
  exit 1
fi
apt-get update
apt-cache policy imagemagick libheif1 libheif-plugin-libde265 libde265-0
apt-get install -y --no-install-recommends \
  'imagemagick=8:6.9.12.98+dfsg1-5.2build2' \
  'libheif1=1.17.6-1ubuntu4.9' \
  'libheif-plugin-libde265=1.17.6-1ubuntu4.9' \
  'libde265-0=1.0.15-1ubuntu0.2' util-linux
convert -version
identify -list format | grep -E 'HEIC|JPEG|PNG|WEBP'
