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

The HttpOnly, host-only rc_session cookie contains a signed opaque handle; user
identity and 24-hour expiry are held in memory. Login rotates handles; logout
revokes them. Restart loses sessions/flows. Google tokens are discarded, never
sent to the frontend or stored in localStorage. Local localhost ports 5173/8080
work with SameSite=Lax and Secure=false. Production HTTPS uses Secure=true;
genuinely cross-site deployment requires SameSite=None with Secure and is subject
to browser third-party-cookie policy. Mutation-origin checks and admin CSRF remain.

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
existing setups and remain readable/searchable.

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
(manufacturer/color), Electronics (motor/esc/servo/gyro/radio).

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
sibling checkouts. APP_URL is required for normal backend server startup and is
independent of ALLOWED_ORIGINS. Successful Google login redirects to APP_URL;
frontend requests send the backend HttpOnly session cookie with credentials.
Frontend JavaScript never receives or stores Google tokens, backend auth tokens
or session handles; the browser manages the HttpOnly cookie.
