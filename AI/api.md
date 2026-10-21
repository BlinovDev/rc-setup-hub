# Implemented API contract

The machine-readable source of truth is `backend/api/openapi.yaml` (OpenAPI 3.1.0),
also embedded verbatim at unauthenticated `GET /openapi.yaml`. It covers the
implemented frontend API; admin HTML is separate. Keep it aligned with handlers,
transport DTOs, status/error mappings and tests. The separate frontend repository
should generate TypeScript types from this contract, not recreate DTOs manually.

JSON application endpoints use `/api/v1`, require a backend-owned session cookie,
and return JSON errors shaped as `{"error":"human-readable message"}` without
internal PostgreSQL/auth details. Profile/resource/error JSON uses
`Cache-Control: no-store`. Browser requests need `credentials: "include"` and an
exact configured CORS origin. No anonymous public browsing exists.

## Health and documentation

- `GET /health`: PostgreSQL ping; 200 `{"status":"ok"}`, 503
  `{"status":"unavailable"}`. No authentication required.
- `GET /openapi.yaml`: 200, `application/yaml`, committed contract. No
  authentication or database query required.

## Authentication and current profile

- `GET /auth/google`: browser navigation; 302 to Google with only openid/email/profile
  scopes, single-use browser-bound expiring state, nonce and PKCE.
- `GET /auth/google/callback`: validates state/nonce/Google identity, upserts by
  stable Google subject, creates a backend session, and returns **303 to APP_URL**.
  The configured frontend URL is required, parsed at startup, and never taken
  from per-request return_to/redirect_uri values. No OAuth tokens/session data
  appear in the redirect. Invalid state is 400; verification failure 401;
  email collision with another subject 409; unexpected failure generic 500.
- `POST /api/v1/auth/logout`: 204, revokes session server-side and expires cookie.
- `GET /api/v1/me`: 200 current profile: id, email, nickname, nullable avatar_url,
  created_at. Provider subject/session internals are excluded.
- `PATCH /api/v1/me`: nickname only, trimmed, non-empty, at most 64 characters,
  no null bytes. Strict application/json object, 4096-byte limit. Success 200
  current profile; invalid input 400; duplicate case-insensitive nickname 409;
  wrong Content-Type 415. Unknown fields are rejected.

First login derives a readable nickname from profile name, then email local part,
then `driver`, keeping letters/digits/hyphens/underscores and replacing spaces with
hyphens (up to 40 characters). A uniqueness collision adds a random suffix with
bounded retries. No frontend onboarding is required; nickname remains editable.

Session lifetime/cookie, token validation, CSRF and origin rules are maintained in
[architecture/security](architecture.md#authentication-and-security). All API calls
remain authenticated and credentialed; token/provider internals are never public.

## Public user profile

`GET /api/v1/users/{user_id}` requires authentication and returns only the existing
PublicProfile fields: id, nickname and nullable avatar_url. Existing user is 200,
invalid UUID 400, valid missing user 404, missing session 401, unexpected database
failure generic 500. No email, Google subject or friendship information is
selected or exposed. This supports reloading a frontend /users/:userId route.

## Nickname discovery

`GET /api/v1/users/search?q=<nickname>`: 200 array of public profiles containing
only id, nickname and nullable avatar_url. Query is trimmed, literal
case-insensitive substring matching on nickname only, max 64 characters without
null bytes. Empty/omitted query returns []; caller excluded; at most 20 rows,
ordered by lower(nickname), id. Percent/underscore are literal. No email search.

## Friendships

- `GET /api/v1/friendships`: 200 `{incoming:[], outgoing:[], accepted:[]}`.
  Items contain relationship id, OTHER user's safe public profile, created_at,
  updated_at. Each group is ordered by updated_at DESC, id DESC; one joined query.
- `POST /api/v1/friendships`: strict JSON `{"user_id":"target-uuid"}` (4096-byte
  limit), requester from session. 201 pending relationship (id, requester_id,
  addressee_id, status, created_at, updated_at). Self/invalid UUID 400, missing
  target 404, existing unordered pending/accepted pair 409, including concurrent
  opposite requests guarded by PostgreSQL uniqueness.
- `POST /api/v1/friendships/{id}/accept`: no body required; only pending addressee
  may accept. 200 accepted relationship with updated timestamp. Requester,
  unrelated or missing returns 404; accepted request returns 409 for addressee.
- `DELETE /api/v1/friendships/{id}`: 204 for either participant. Pending requester
  cancels, pending addressee rejects, accepted participant removes. Row is
  physically deleted; unrelated/missing returns 404; later request is possible.

Only pending/accepted states exist. Setup access reacts immediately to database
state; pending grants nothing and deletion removes friends-only access.

## Active chassis selection

- `GET /api/v1/chassis/brands`: 200 active values, array of `{id,name}`.
- `GET /api/v1/chassis/brands/{brand_id}/models`: 200 active models under an active
  brand, array of `{id,name}`; invalid UUID 400, unknown/inactive brand 404.

Client may show synthetic "Custom / not listed" and send chassis_model_id=null.
There is no Custom database row. Inactive historical values remain attached to
existing setups and remain readable/searchable. Admin catalog names are trimmed,
1–100 Unicode characters without controls; Custom is reserved case-insensitively.
Models require an active parent; brand_id is immutable on rename. Forms retain
8 KiB limits, CSRF and 303 success redirects; duplicates 409, malformed 400, missing 404.

## Setup CRUD and lists

- `POST /api/v1/setups`: 201 complete Setup plus relative Location header.
  Required title, visibility, data; optional nullable chassis_model_id and notes.
  Owner comes from session, schema_version is always 1. Non-null model must exist
  with active model and brand. Title trimmed/non-empty/max 150 characters; notes
  max 10,000 characters; technical strings trimmed/max 200. No null bytes in text.
- `GET /api/v1/setups/{id}`: 200 complete Setup when visible. Owner sees all;
  public is visible to authenticated users; friends to accepted friends; private
  owner-only. Unknown/inaccessible uses identical 404 before protected data decode.
- `PATCH /api/v1/setups/{id}`: owner-only, 200 complete Setup. Omitted top-level
  fields are unchanged; explicit null clears chassis_model_id or notes only.
  Title, visibility and data cannot be null. Supplied data replaces the entire
  schema-v1 document, never deep-merges. Empty object is allowed. Unchanged
  historical inactive chassis is allowed; changed non-null chassis must be active.
  Concurrent update may return 409; reload before retry. updated_at is set on writes.
- `DELETE /api/v1/setups/{id}`: owner-only, 204. Missing/not-owned returns 404.
- `GET /api/v1/me/setups`: 200 all owned setups regardless of visibility.
- `GET /api/v1/users/{user_id}/setups`: 200 visible target-user setups: owner all,
  accepted friend public+friends, other/pending public only. Valid unknown user
  returns []. Both lists order created_at DESC, id DESC, without pagination.

Setup JSON exposes id, owner_id (application UUID), chassis_model_id (nullable),
chassis (nullable read-only display), title, visibility, data, notes (nullable),
schema_version, created_at and updated_at. Resolve the owner profile through
GET /api/v1/users/{owner_id}; owner email/provider identity is never included.
Chassis display contains model_id, model_name, brand_id and brand_name, including
inactive historical catalog values. Custom/unlisted setup has both
chassis_model_id=null and chassis=null. Detail/list/create/PATCH responses share
this shape. Reads and mutation results join catalog metadata; list retrieval uses
one query without per-setup lookups. Active-only selection rules remain unchanged.
Visibility is exactly public/friends/private. Create/PATCH are strict
application/json objects, max 256 KiB; malformed/unknown fields (including
owner_id/schema_version) are 400. Write authorization remains owner-only even
for accepted friends/public readers. Authentication absence is 401; invalid
UUID/input 400; internal errors generic 500. Untrusted browser mutation origin 403.

### Technical schema v1

Explicit reusable structures: SetupDataV1, Suspension (front/rear), AxleSuspension
(camber_deg/caster_deg/toe_deg/link_lengths), LinkLength (name/length_mm), Shocks
(front/rear), Shock (manufacturer/model/spring/oil_cst), Spring
(manufacturer/color), Electronics (motor/esc/servo/gyro/radio), and optional
rear_differential (type gear/lsd/ball/spool, manufacturer_id referencing the existing
chassis brands catalog, model, tuning_notes). Differential fields are independent:
model uses the 200-character technical limit; tuning notes are trimmed free text,
max 10,000 characters, with no null bytes. Null/empty differential blocks are omitted.
New manufacturer selections must exist and be active; an unchanged historical
inactive reference survives PATCH, including full-data replacement. Full Setup responses
include read-only rear_differential_manufacturer ({id,name} or null), joined once with
no activation filter so historical names remain displayable; it is never stored in
technical JSONB or accepted in requests. Clearing/removal
uses null/omission in the replacement data document. This additive schema-v1 JSONB
extension needs no migration/backfill; old records receive no defaults. Older clients
must preserve the entire data document to avoid dropping fields on replacement; the
companion frontend consumes the exact updated backend contract before using them.

Either Shock also accepts optional piston {holes, hole_diameter_mm, notes} and seal
{type, description}. All fields are independent. Hole count is a positive int32;
diameter is finite and positive in mm without rounding. Piston notes are trimmed
free text up to 10,000 characters; seal description is technical text up to 200;
no null bytes. Seal type is o-ring/x-ring or null/omitted. Empty/null blocks and
parents introduced solely by them are pruned; existing legacy empty shock/spring
shapes stay unchanged. This additive schema-v1 extension needs no migration/backfill.
Existing shock manufacturer/model/spring/oil values and their rules are unchanged.

All sections and most technical fields are optional. Omitted/null optional
numeric fields mean unknown and are omitted on output; **toe_deg: 0 is a real
value preserved by pointer-backed Go structs**. Optional null sections/technical
strings and empty strings/lists are omitted on output. Link entries require a
trimmed non-empty name and positive length_mm (max 100 per axle); supplied oil_cst
must be positive. Angles are finite without narrow drift-domain bounds. Detail
schema_version is 1; clients cannot choose it. No version history exists.

## Public setup discovery

`GET /api/v1/setups/search`: authenticated and **public only**, even for the owner
or an accepted friend. SQL filters visibility before any protected data decoding.

Parameters:
- q: trimmed literal case-insensitive substring of title OR owner nickname,
  max 100 characters without null bytes; empty means no text filter. Does not
  search email, notes, Google identity or JSONB; percent/underscore are literal.
- brand_id/model_id: UUID filters combined with AND. Valid nonexistent/mismatched
  values return zero results; invalid UUID 400. NULL chassis does not match.
  Inactive historical brands/models still match and show names.
- limit: integer 1..50, default 20.
- cursor: opaque next_cursor from preceding response; malformed value 400.
  Repeat q/brand_id/model_id and desired limit on subsequent requests.

Response 200 `{ "items": [], "next_cursor": null }`. Items contain id, title,
visibility=public, chassis_model_id, schema_version metadata, timestamps, owner
safe public profile and nullable chassis projection (model_id/model_name/
brand_id/brand_name). No email, technical data or notes. One joined query avoids
N+1. Newest-first total order is created_at DESC, id DESC with keyset pagination;
limit+1 determines next_cursor without COUNT. Cursor encoding is not a client
contract. Complete setup data comes from the detail endpoint.

## Admin HTML (outside frontend OpenAPI)

Existing routes: GET /admin and /admin/users; GET/POST /admin/chassis/brands and
/admin/chassis/models; POST each catalog /{id}/update, /disable and /enable.
All require authentication (401 otherwise) and ADMIN_EMAILS allowlist (403 for
non-admin). Forms require existing CSRF protection; HTML uses no-store and
buffered escaped templates. Dashboard counts all users/all setups/public setups;
users table shows nickname/email/UTC created timestamp/all-owned setup count.
No database admin role or frontend SPA exists.


## Frontend contract delivery

The frontend is a separate React application/repository and may live anywhere.
Copy backend/api/openapi.yaml into the frontend repository's api/openapi.yaml,
or obtain the same contract from a running backend at GET /openapi.yaml. Commit
the frontend's own copy for reproducible TypeScript generation. Do not assume
sibling checkouts. Security/configuration remain in [architecture](architecture.md#authentication-and-security);
frontend integration rules are owned by its API document.

## Arm spacing extension (arm-spacing-v1)

`SetupDataV1.suspension.front/rear` each support optional `top_arm_spacing` and
`lower_arm_spacing`, using reusable `ArmSpacing` with pointer-backed optional
`front_mm` and `rear_mm`. The outer front/rear identifies the axle; inner
front/rear means ahead of/behind the arm in vehicle direction. Values are total
shim thickness in mm, symmetric left/right. Both blocks are independently
optional on either axle. Example: `{"suspension":{"front":{"top_arm_spacing":
{"front_mm":0,"rear_mm":5.2}}}}`.

Numbers must be finite and >=0. JSON numeric strings, unknown fields, malformed
values and negatives are rejected without writes. Zero remains zero, null or
omission remains unknown; both unknown values omit the block and parents
introduced solely for it. No rounding/defaults/calculations occur. PATCH still
replaces supplied data in full; omit a block to remove it, omit/null one value
to clear it while retaining its sibling. Omit top-level data for unrelated edits.
Existing schema-v1 documents without spacing remain unchanged and need no defaults.

## Setup photo APIs (Issue #5)

All routes use the existing credentialed cookie/session and mutation-origin
checks. Multipart uploads have their own bounded parser; technical data remains
strict 256 KiB JSON/full-replacement PATCH. Never include photos in technical data.

- GET /api/v1/setups/{id}/photos: 200 SetupPhoto array, stable created_at/id order.
- POST same path: owner-only multipart file + UUID request_id, exactly one file.
  201 SetupPhoto. File maximum 25 MiB; whole multipart <=25 MiB+64 KiB. Accepted
  JPEG/PNG/WebP/HEIC/HEIF are detected by content, never filename/MIME. First frame
  becomes an oriented stripped JPEG with bounds 2400/640px. Maximum ten photos.
  Same request_id and bytes returns the existing photo without duplicate writes;
  different bytes or retry after deleting that photo returns 409. Receipts last
  until setup deletion. Use a new UUID for an intentionally new upload.
- PUT /api/v1/setups/{id}/photos/{photo_id}/cover: owner-only, no body, 200 SetupPhoto.
- DELETE /api/v1/setups/{id}/photos/{photo_id}: owner-only, 204; cover fallback is
  oldest remaining. File deletion failures retain a durable cleanup queue.
- GET .../{photo_id}/image or /thumbnail: authenticated authorized processed JPEG,
  Cache-Control: no-store, private; nosniff; no ETag/Last-Modified or 304 cache reuse.

401 missing session; 403 untrusted mutation origin; 400 invalid UUID/multipart,
unsupported/corrupt/oversized image or ten-photo limit; 404 missing/inaccessible
setup/photo or non-owner mutation; 409 idempotency conflict; 503 upload capacity
busy (two concurrent uploads per process); 500 processing/storage/DB failure,
retry using the same request_id. Owner authorization precedes decoding; media
visibility filters run before any file access and are rechecked before delivery.
Prior downloads/in-flight authorized responses cannot be revoked retroactively.

Setup and SetupSearchItem include cover_photo: SetupPhoto|null in the same joined
query (including My setups and visible profile lists). Schema-v1 is unchanged.
SetupPhoto exposes ID/setup ID, full/thumbnail dimensions, image/jpeg, is_cover,
created_at and authenticated relative url/thumbnail_url. Generated storage keys
and filesystem paths are never exposed. Gallery is a separate resource.
Refresh gallery/detail/list/search cover state after immediate photo mutations.
New frontend creation saves once, uploads against returned ID with per-file
results/retry; never create a second setup due to image errors.

## Square crop contract (Issue #10; roadmap stage 2)

PUT /api/v1/setups/{id}/photos/{photo_id}/crop is owner-only JSON with exactly
expected_revision (integer 0..2147483647) and crop (null or {x,y,size}). All three
crop fields are required integers: x/y >=0, size>0; x+size<=stored full width and
y+size<=stored full height. Coordinates refer to the oriented/resized full JPEG,
never original upload or client dimension claims. Null resets to size=min(w,h),
x=floor((w-size)/2), y=floor((h-size)/2). Odd center offsets round down. Thumbnail
uses ImageMagick crop/+repage then 640x640> JPEG quality 85, no upscaling; full bytes
never change. FE preview uses the authorized full JPEG and these exact coordinates;
allow JPEG/resampling differences, not a different source or coordinate frame.

200 SetupPhoto includes nullable crop and thumbnail_revision in gallery, upload,
cover/crop responses and all cover projections. Legacy crop=null/revision=0;
new centered-square photos start at revision=1. thumbnail_url includes
?revision=N as query identity; the endpoint always serves CURRENT authorized media,
not historical versions. FE media keys must include photo ID + thumbnail_revision,
invalidate loaded thumbnail blobs and list/detail/gallery cover state after apply.
Full-photo blob identity does not change for crops; no metadata N+1 is needed.

Apply requires the current revision. Identical current-revision requests are
no-ops. One-step-old identical retries return the already committed photo safely;
other stale revisions return 409 and require reload before explicit retry. There
is no automatic retry. 400 malformed/missing/null coordinate, unknown field,
fraction/string/overflow/out-of-bounds; 401 session; 403 mutation origin; 404 missing/
not-owned/deleted; 500 processing/storage/DB failure; 503 shared processor busy.
Owner checks precede parsing/processing. Crop/cover/delete share the parent setup
lock through generation/commit; deletion cannot resurrect media. GET privacy and
no-store rules remain in force regardless of revision query or known URLs.

## Photo upload error details (Issue #14)

The existing APIError.error string now distinguishes these 400 reasons:
`invalid photo: unsupported image format`, `invalid photo: image could not be decoded`,
`invalid photo: file must be nonempty and at most 25 MiB`,
`invalid photo: image must be at most 50 megapixels`.
Multipart byte/empty rejection remains `file exceeds 25 MiB or is empty`;
count remains `a setup may have at most 10 photos`. Clients should translate only
known strings and retain a generic fallback. No new DTO/status is required.
Processor slot exhaustion maps to 503; termination/timeouts/invocation failures
map to generic 500. Logs contain stage/format/exit status only, never raw decoder
stderr, filename, image bytes, paths or EXIF. Reverse proxies may return non-JSON
413 independently; the frontend should explain that request-size rejection.

## Front steering knuckle

Optional schema-v1 `front_steering_knuckle` contains independent `manufacturer_id`
(existing chassis brand UUID) and trimmed `description` (part number/product text,
max 200 Unicode characters, no null bytes). Empty/null blocks prune; no rear block,
defaults or migration. New manufacturer selection requires an active brand;
unchanged historical references survive full-data PATCH. Read-only
`front_steering_knuckle_manufacturer` joins historical catalog names on detail/list
and mutation responses, including inactive brands, without extra per-item lookups.
