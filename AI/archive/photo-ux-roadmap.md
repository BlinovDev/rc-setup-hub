# setup-photo-ux-v2 roadmap

Linked backend Issue #10 and frontend BlinovDev/rc-setup-hub-fe#13. Follow small gates:

1. FE UI foundation: manager above fields, compact paperclip/count/info toolbar.
   Gate: RTL and 375px/desktop browser, queue/limits/no duplicate save or form reset.
2. BE crop contract (this PR): normalized integer framing, crop/revision persistence,
   owner mutation, immutable publication/cleanup, joined projections/legacy handling.
   Gate: coordinates/full checksum/idempotency/concurrency/authorization/OpenAPI,
   real PostgreSQL/full/race/native ARM64 tests. Record exact PR head/contract hash.
3. FE editor: consume exact BE PR contract, server-normalized full preview for all
   formats, pan/zoom/touch/keyboard/Apply/Cancel/Reset and explicit retry.
   Gate: shape/orientation fixtures, matching frame, no draft persistence on cancel,
   reload persistence, loaded blob/projection invalidation by thumbnail revision.
4. FE list/gallery polish + paired QA: bounded square mobile rows/desktop cards,
   existing WebP placeholder, uncropped full viewer; browser 320/375/390/768/1280.
   Gate: long titles, errors, no-photo, actions/focus, physical device/live paired QA.

This backend delivery implements stage 2 only; FE stages and paired live QA remain
pending. Record exact compatible BE/FE SHAs on linked PRs after FE consumption.
No merge/deployment/live server work is authorized here; production waits reviewed QA.
