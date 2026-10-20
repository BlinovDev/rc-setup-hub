# Issue #14: photo upload investigation and staging verification

## Evidence and ownership

Inspected backend main `50f82c88c51be797caf9ecfd6e7cb3d381d706a8` and frontend main
`17eb5bd28828ebfdd88f203e7861f55119f232bd`, their AGENTS/AI documentation, native
processor, multipart handler, upload queue/client and deployment configuration.
Related photo work: backend #5/#10 and frontend #11/#13 (implemented); no existing
open fix duplicates #14.

The frontend sends the original File unchanged in browser-generated multipart;
selection/crop does not transcode it. Crop uses the saved normalized JPEG after
upload. The backend selects the decoder from magic bytes, ignoring filename and
file MIME. A HEIF-to-JPEG filename change alone therefore does not explain rejection.
The server README documents nginx `client_max_body_size 26m` and 120s read timeout;
the API allows 25 MiB plus 64 KiB multipart overhead. Live nginx configuration and
the incident's exact HTTP response/bytes were not available, so infrastructure or
a particular iPhone HEIF codec variant cannot be ruled out.

A concrete pipeline defect was reproduced: a generated 4284x5712 grayscale JPEG,
under 25 MiB and 50 MP, passes signature/bounds checks but fails normalization.
On Ubuntu ImageMagick Q16, full-resolution decoding/cache cloning spills a cache
larger than the process's 64 MiB RLIMIT_FSIZE; the decoder exits 25 (SIGXFSZ).
The original processor collapses that into ErrImage/HTTP 400. The frontend discards
all 400 response reasons and displays the same format/size/count message.
The new TestPhoneResolutionJPEG fails on the original processor and passes with
JPEG decoder-side scaling. A square jpeg:size hint still decodes unnecessarily
large caches; the hint must match the validated source aspect ratio.

## Changes and limits

JPEG decode uses a proportional <=2400px hint AFTER original-dimension validation,
then the existing orientation/resize/strip/JPEG and square-thumbnail path. Limits
remain 25 MiB, 50 MP, ten photos; address space, CPU, 64 MiB file size, decoder
memory/map/disk, wall time and concurrency remain unchanged. HEIF content sniffing
and native decoding remain fail-closed; no extension/MIME whitelist bypass, new
codec, dependency, migration or server provisioning change.
Safe validation reasons now distinguish unsupported content, decode failure,
bytes and dimensions; count validation already had a specific reason. Decoder
failure logs contain only stage, detected format, byte count, known dimensions
and cancellation state, never filenames, image bytes, EXIF, native stderr or paths.
Busy processing returns retryable 503. Frontend retains bounded 400 validation
reasons, handles even HTML proxy 413 responses explicitly, and keeps generic
500/network errors without exposing backend internals. UUID/byte retry semantics
and setup/unsaved-field preservation are unchanged.

## Fixture and verification limits

The existing 64x64 libheif HEVC fixture is genuine encoded HEIF, not an iPhone
capture. Tests cover native HEIF -> stripped JPEG -> re-upload, HEIF-compatible
brands/AVIF exclusion, misleading upload metadata, synthetic phone-resolution
JPEG and original >50 MP rejection before shrink, corrupt/unsupported/oversized
input, and frontend multipart MIME/name variants. Existing orientation, privacy,
count, crop, cleanup and retry suites remain required. These fixtures do not
establish all iPhone 16 HDR/auxiliary-image variants or real Safari transcoding.
The incident's IMG_4144 bytes were not supplied; the screenshots refer to different
filenames. The reproduced defect is confirmed, but attribution of that exact
production rejection and physical iPhone acceptance remain pending.

## Staging verification (no deployment performed)

1. After required CI passes, separately authorize a paired staging release with
   exact backend/frontend PR head SHAs via the backend deploy workflow. Do not
   replace another review deployment without coordinating the shared environment.
2. On a physical iPhone in Safari, sign in and create/edit a disposable setup.
   Select the originally failing photo from Photos. Also test an exported JPEG,
   raw HEIF/HEIC, rotated portrait, landscape, PNG and WebP. Capture filename,
   file.type, byte size, multipart Content-Type, response status and safe JSON
   reason via Safari remote inspector. Never attach originals/EXIF/user data to
   public logs. Correlate safe decoder stage/format/size/dimensions in server logs.
3. Expect 201 for supported valid files, full JPEG <=2400px with correct orientation,
   stripped metadata and square thumbnail <=640px. Open full image, adjust/reset
   crop, change cover, reload and inspect Home/My/profile/detail projections.
4. Test malformed/spoofed content, >25 MiB, >50 MP original and an eleventh photo:
   reject with the corresponding reason and no published orphan or limit bypass.
   Confirm unrelated text edits remain unsaved/intact through photo failures.
5. Interrupt and retry the same queued file: UUID/bytes unchanged and one photo
   only. Busy/server/network failures must remain retryable. Exercise 413 and
   400 responses separately: UI must not label a proxy limit as a format error.
6. Verify public/friends/private using two accounts and known media URLs; revoked
   friendship/private changes must still deny reads. Confirm no-store/nosniff.
7. If the exact iPhone file still fails, record its response and detected format/
   stage before choosing another fix. Check the actual staging nginx 26m/120s
   settings and pinned native decoder version/plugin. Do not loosen security
   limits or claim an unsupported HDR/auxiliary-image variant was tested.

No merge, deploy, workflow dispatch or production modification is authorized here.
