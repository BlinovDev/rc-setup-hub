phone-format.heic is libheif's 64x64 HEVC still-image decoder fixture
fuzzing/data/corpus/colors-no-alpha.heic, from strukturag/libheif commit
f223c23c5863cd9e7f29c48438c4da3b308a17aa. Source:
https://github.com/strukturag/libheif/blob/f223c23c5863cd9e7f29c48438c4da3b308a17aa/fuzzing/data/corpus/colors-no-alpha.heic
The upstream COPYING with LGPL/MIT notices is included. This verifies HEVC/HEIF conversion,
not a physical iPhone capture, all iPhone codec/profile variants or device QA.

phone-24mp.heic is a synthetic 4284x5712 grayscale gradient created for Issue #14,
not a personal photograph or a camera capture. It exercises phone-resolution HEVC
HEIF decoding and temporary-cache behavior. Generated from ImageMagick's built-in
gradient via JPEG, then ImageMagick 6.9.12-98/libheif 1.17.6/x265 HEIC at quality 40
with heic:speed=9. No external copyrighted imagery, metadata or personal data.
The generated fixture is dedicated to the public domain (CC0-1.0).
