# Issue #14: photo upload investigation

Inspected backend main 50f82c8 and frontend main 17eb5bd, root/nested AGENTS,
relevant AI docs, photo selection/queue/API/error mapping, multipart handler,
signature detector, ImageMagick processor, decoder provisioning and deployment docs.

## Confirmed defect and evidence

The frontend sends the original File through FormData, without canvas/export or
MIME override. The backend selects JPEG/PNG/WebP/HEIC from signatures, not extension
or multipart MIME. A HEIF file called .jpeg is not inherently rejected.

The bounded native decoder previously used RLIMIT_FSIZE=67108864 (64 MiB), while
ImageMagick's disk-cache budget was already 512 MiB. RLIMIT_FSIZE covers temporary
uncompressed pixel caches as well as final JPEGs. A generated 4284x5712 JPEG of
about 222 KiB passes format and dimension inspection, but conversion exits 25
(SIGXFSZ handled by ImageMagick) under the old limit. The same conversion succeeds
with a 512 MiB per-file ceiling and produces an 1800x2400 JPEG. The new Go regression
fails against the original processor and passes with the fix and native HEVC plugin.
The old processor mapped all conversion failures to 400; the frontend discarded
all error details and displayed one generic format/size/count message.

This establishes a backend image-processing defect consistent with the reported
24.47 MP image. The original rejected bytes, production response and server logs
are unavailable; it does not establish that the exact production upload reached
this branch. The two screenshot filenames differ. No claim of an iPhone-origin
fixture or physical Safari verification is made.

HTTP infrastructure: repository host instructions require nginx client_max_body_size
26m, above the 25 MiB + 64 KiB multipart cap. No live nginx configuration or request
trace was available. No speculative proxy change is made; frontend handles 413.

## Fix and safety

Align subprocess file size with the existing 512 MiB ImageMagick disk-cache budget;
retain 64 MiB processed-output publication checks (including crop publication).
Keep 25 MiB input, 50 MP, ten photos, 1 GiB address space, 20 CPU seconds, 30-second
processing deadline, two processor slots, 256 MiB memory/map caches, allowed content
signatures, first-frame behavior, auto-orientation, metadata stripping, private
paths and authorization unchanged. Resource/process termination is a generic server
failure; known validation reasons remain 400 and processor capacity is 503.
Only safe stage/format/exit fields are logged. No decoder stderr/EXIF/bytes are logged.

## Staging verification (not executed; no deployment authorized)

1. After both PR checks pass, explicitly select their backend/frontend SHAs in the
   existing combined staging workflow. Record the pair and confirm matching OpenAPI.
   Follow existing media preflight/backup/health gates. Do not use production data.
2. On a physical iPhone, sign in on staging. Upload the original failing photo from
   Photos; record selected filename/type/size, HTTP status and safe validation text.
   Repeat native HEIF selection and a JPEG export, portrait and landscape. Verify
   preview, successful upload, normalized orientation, full image, square thumbnail,
   cover/list/detail, crop apply/reset, reload and retry without duplicates.
3. Upload a valid 4284x5712 JPEG, PNG and WebP. Confirm originals/EXIF are not served.
   Include a HEIF with .jpeg metadata and JPEG with .heic metadata via a test client;
   actual bytes determine acceptance. Reject SVG/AVIF/corrupt bytes, >25 MiB,
   >50 MP and an eleventh photo with the appropriate reason. Keep the same request_id
   when retrying the same bytes. Inspect 413 separately if the proxy blocks a request.
4. Check busy/server/network failures retain setup, completed uploads and retry UUID;
   no raw native error appears in UI or logs. Verify layout at 375px and desktop.
5. With two staging users, verify private/friends visibility, unfriend revocation,
   owner-only mutations and no-store full/thumbnail responses. Verify crop/cover
   changes preserve unsaved technical fields. No production deployment is included.

## High-resolution JPEG follow-up

A generated 50 MP JPEG additionally exhausted the unchanged native cache budget
when decoded at full resolution. JPEG's existing native decoder size hint now
requests 2400x2400 before conversion; original dimensions are still inspected
without that hint and rejected above 50 MP. `TestMaximumResolutionJPEG` verifies
successful processing exactly at the 50 MP boundary without raising cache/memory
budgets. This also reduces work for iOS high-resolution JPEG exports. HEIF decoding
continues through the existing pinned native codec and bounded resources.
