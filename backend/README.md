# Backend — Phase 7

## Local PostgreSQL

Run these commands from `backend/` with Docker running:

```sh
docker compose up -d --wait
export DATABASE_URL='postgres://rc_setup_dev:rc_setup_dev@127.0.0.1:5432/rc_setup_hub?sslmode=disable'
go run ./cmd/migrate up
# Set the Google/session variables shown below before starting the server.
go run ./cmd/server
curl -i http://localhost:8080/health
```

The Compose credentials are development-only. PostgreSQL 17 listens only on
localhost. Its named volume persists data across `docker compose down`.
Stop PostgreSQL with `docker compose down`; start it again with
`docker compose up -d --wait`.

## Configuration and health

`DATABASE_URL` is required. The server verifies the pgx pool connection before
listening and exits with a nonzero status if configuration or connectivity fails.
Database URLs and credentials are not logged. PostgreSQL sessions use UTC.

`HTTP_ADDR` defaults to `:8080` when unset or empty.

`GET /health` pings PostgreSQL with the request context and a two-second timeout.
It returns JSON `{"status":"ok"}` with HTTP 200 when connected, or
`{"status":"unavailable"}` with HTTP 503 when the database cannot be reached.
Database errors are not included in the response. Health checks connectivity,
not migration status; apply migrations before starting the server.

Request flow: request logging, panic recovery, chi routing, database ping,
JSON response. Logs use UTC timestamps and include method, path, status and
duration (nanoseconds). SIGINT/SIGTERM allows active requests up to ten seconds
to finish; the database pool is closed after normal HTTP shutdown.

## Migrations

The goose SQL files are embedded in the migration runner:

1. `00001_users.sql`: users and case-insensitive email/nickname uniqueness.
2. `00002_friendships.sql`: friendships, status/self checks, unordered-pair
   uniqueness and requester/addressee indexes.
3. `00003_chassis_brands.sql`: brands and case-insensitive name uniqueness.
4. `00004_chassis_models.sql`: models, per-brand case-insensitive uniqueness,
   brand index and restricted brand deletion.
5. `00005_setups.sql`: setups, visibility/version checks and all initial indexes.

`go run ./cmd/migrate up` applies outstanding migrations.
`go run ./cmd/migrate down` reverts the latest migration. Down migrations drop
that table and its data; run them only when that data loss is intended.
Server startup does not apply migrations automatically.

IDs default to PostgreSQL `gen_random_uuid()`. Both timestamp columns default to
`now()`; future repositories must explicitly update `updated_at` on writes.
User deletion cascades to friendships and setups. Model deletion sets a setup's
model reference to NULL. Disable catalog entries during normal operation;
physical model deletion is supported as documented in the database design.
The user service normalizes email before persistence. Active-catalog selection
rules belong to later catalog/setup services. Migrations enforce database
uniqueness, foreign keys, NOT NULL and check constraints without triggers.
No catalog data is seeded and no JSONB GIN index is created.

The migration provider uses the
[goose Provider API](https://pressly.github.io/goose/documentation/provider/)
with pgx's database/sql adapter; normal application access uses pgxpool.

## Tests

```sh
go test ./...
```

With Docker running, integration tests automatically start a disposable
`postgres:17-alpine` container on a random localhost port, wait for connectivity,
and apply every migration to its empty database. Each constraint case uses a
transaction that is rolled back. Tests also verify migration rollback/reapply,
indexes, UUID/TIMESTAMPTZ types, defaults, JSONB, foreign keys and deletion rules.
The pool and container are cleaned up when the suite completes. Tests never
connect to the development database or rely on manually created tables.
The reusable helper is `internal/database/dbtest.New`.

Docker unavailability fails the integration suite. To explicitly run only tests that do not need PostgreSQL, use
`go test -short ./...`. The authenticated HTTP suite also uses real PostgreSQL.
Google accounts and live Google requests are never required by the automated tests.


## Google login and sessions

Create a Google OAuth client of type **Web application**, configure its consent
screen, and add your Google account as a test user if the app is in testing mode.
Register this exact authorized redirect URI for the documented local setup:

```text
http://localhost:8080/auth/google/callback
```

Run these exports in the same shell as the server (substitute your own OAuth
credentials; do not commit them):

```sh
export HTTP_ADDR=':8080'
export DATABASE_URL='postgres://rc_setup_dev:rc_setup_dev@127.0.0.1:5432/rc_setup_hub?sslmode=disable'
export GOOGLE_CLIENT_ID='YOUR_CLIENT_ID.apps.googleusercontent.com'
export GOOGLE_CLIENT_SECRET='YOUR_CLIENT_SECRET'
export GOOGLE_REDIRECT_URL='http://localhost:8080/auth/google/callback'
export SESSION_SECRET="$(openssl rand -base64 32)"
export SESSION_COOKIE_SECURE='false'
export SESSION_SAME_SITE='lax'
export ALLOWED_ORIGINS='http://localhost:5173'
docker compose up -d --wait
go run ./cmd/migrate up
go run ./cmd/server
```

`SESSION_SECRET` must be base64 encoding of at least 32 cryptographically random
bytes. Generate it once for the running deployment. Google credentials and this
secret are required for the server, but migrations still require only DATABASE_URL.
`SESSION_COOKIE_SECURE` defaults to false for local HTTP; enable it for HTTPS.
HTTPS redirect configuration requires Secure cookies. `SESSION_SAME_SITE` defaults
to `lax`, which supports a separate client on localhost:5173 and backend on
localhost:8080 (same site, different origins). Use the same hostname in both.
For genuinely cross-site HTTPS clients, use `none` with Secure enabled.
`ALLOWED_ORIGINS` is optional, comma-separated, and accepts exact HTTP(S) origins
only. There is no wildcard or credentialed CORS default. Browser clients call
these APIs with credentials included; untrusted mutation origins are rejected.

Open `http://localhost:8080/auth/google` in a browser. The backend redirects to
Google with only `openid email profile`, random state and nonce, and an S256 PKCE
challenge. State is browser-bound through a signed HttpOnly cookie, expires after
ten minutes, and is consumed atomically once. Callback denial and exchange
failure also consume valid state. On callback, the server exchanges the code
with the stored PKCE verifier and verifies the Google ID token signature,
issuer, audience, expiration, nonce, verified email and authorized-party claims.
When supplied, the access-token hash is also checked. This follows the
[Google OIDC validation requirements](https://developers.google.com/identity/openid-connect/openid-connect).

Successful authentication upserts by Google subject, updates normalized email
and avatar, and preserves the existing ID/nickname. Email collisions with a
different subject fail with HTTP 409. New nicknames derive from the profile name,
then email local part, then `driver`. The candidate retains letters, digits,
hyphens/underscores, replaces spaces with hyphens, and is limited to 40 characters.
A case-insensitive collision adds a random suffix and retries up to five times.
There is no onboarding screen.

Application sessions use Gorilla securecookie to authenticate a cookie containing
only a cryptographically random opaque handle. The user ID and session expiry
stay in a bounded in-memory store, separate from OAuth flow state. Cookies are
HttpOnly, host-only, and live for 24 hours. Login rotates any existing handle;
logout revokes it server-side and expires the cookie, so replay after logout fails.
Google tokens are discarded after verification, never placed in cookies or logs.
This single-process POC intentionally loses sessions and pending flows on restart.
Multiple replicas or persistent sessions would require a later accepted design.

After login, the callback redirects to `/api/v1/me`, which displays your JSON
application profile. No frontend is implemented.

## Phase 3 API contract

- `GET /auth/google`: 302 to Google authorization.
- `GET /auth/google/callback`: validates state and identity; successful login
  creates a session and redirects with 303 to `/api/v1/me`. Invalid state returns
  400; failed identity verification returns 401. Provider/DB details are hidden.
- `GET /api/v1/me`: authenticated profile with `id`, `email`, `nickname`,
  nullable `avatar_url`, and UTC `created_at`. Google subject, session handles,
  and OAuth tokens are excluded. Missing, expired or revoked sessions return 401.
- `PATCH /api/v1/me`: requires `Content-Type: application/json` and a body such as
  `{"nickname":"My driver name"}`. Only nickname is accepted, up to a 4 KiB body.
  Whitespace is trimmed; empty/over-64-character nicknames, unknown fields,
  malformed/trailing JSON and null characters return 400. Duplicate nicknames
  return 409. Success returns the updated profile. Maximum length is measured
  in Unicode characters, not bytes.
- `POST /api/v1/auth/logout`: authenticated request, returns 204, revokes the
  session and deletes the cookie. A later `/me` request returns 401.

For nickname updates from the browser developer console after signing in:

```js
await fetch('/api/v1/me', {
  method: 'PATCH', credentials: 'include',
  headers: {'Content-Type': 'application/json'},
  body: JSON.stringify({nickname: 'Track driver'})
}).then(r => r.json());
await fetch('/api/v1/auth/logout', {method: 'POST', credentials: 'include'});
```

All profile/error responses are JSON with Cache-Control: no-store. The normal
suite tests the Google flow with a fake provider, OIDC verification with locally
signed tokens/local JWKS, and user persistence against disposable PostgreSQL.


## Phase 4 admin shell

Admin authorization is an environment-only allowlist. To allow your signed-in
Google account, set its email exactly as shown by `GET /api/v1/me`:

```sh
export ADMIN_EMAILS='your-google-email@gmail.com'
go run ./cmd/server
```

Replace the example address with your current account's email and keep the
existing Google/session/database variables from the instructions above. Restart
the backend after changing the allowlist; then sign in again and open
`http://localhost:8080/admin`. Multiple accounts can be configured as:

```sh
export ADMIN_EMAILS='first@example.com, second@example.com'
```

Parsing trims surrounding whitespace, lowercases addresses, and ignores empty
items. Matching is case-insensitive. Empty/unset configuration grants nobody
admin access. A malformed address (including display-name syntax) fails server
configuration rather than allowing a partial list. No admin flag, role, table,
or migration is involved.

Admin request flow is the existing session/user authentication middleware,
email allowlist authorization middleware, admin-only CSRF middleware, then the
HTML handler. Unauthenticated requests receive HTTP 401, consistent with the
existing authentication behavior; sign in through `/auth/google`. Authenticated
non-admin users receive 403; allowlisted users receive 200. The shell displays the application title and nickname/email. Phase 5 adds
Brands and Models navigation and forms; Users remains a placeholder. Statistics
and a users listing are not implemented.

The embedded `templates/admin/index.html` is rendered with `html/template`
autoescaping. Output is buffered before writing, so an execution failure returns
a generic HTTP 500 without partial HTML or template details. Admin pages are
marked Cache-Control: no-store.

[Gorilla CSRF](https://github.com/gorilla/csrf) protects the entire admin route
group. It uses the configured SESSION_SECRET and a separate signed HttpOnly
`rc_admin_csrf` cookie scoped to `/admin`, with SameSite=Lax and the existing
Secure setting. GET does not require a mutation token. Future POST/PUT/PATCH/DELETE
handlers registered in the group automatically require a valid token and pass
same-origin validation. Local HTTP is explicitly identified to the library;
HTTPS retains its strict Origin/Referer checks. No cross-origin admin trust is
added, and the JSON API has no new browser-form CSRF requirements.

Future server-rendered forms can place `{{.CSRFField}}` inside their form element.
The reusable `admin.PageData.CSRFField` must be populated with
`csrf.TemplateField(r)` from a request that passed through admin CSRF middleware.
This library-generated hidden input is the only trusted HTML field; user profile
strings continue to be escaped normally. The middleware also accepts the standard
X-CSRF-Token header. Tests cover valid form/header tokens, all mutation methods,
missing/invalid tokens, missing cookies, untrusted origins, and HTTP/HTTPS settings.

Admin access tests use the existing real PostgreSQL harness, actual application
sessions, and a fake Google provider. Normal tests require no Google account.
Run both `go test ./...` and `go test -race ./...` with Docker available.


## Phase 5 chassis catalog

The backend uses the existing chassis_brands/chassis_models tables; no migration
or catalog seed records were added. Domain types, explicit pgx SQL, validation,
and HTTP handlers live in `internal/chassis/{types,repository,service,handlers}.go`.
The application wiring registers catalog routes inside the existing authenticated
API group and authenticated/admin/CSRF admin group.

Admin pages:

- `GET /admin/chassis/brands`: all brands, create, rename, enable/disable forms.
- `POST /admin/chassis/brands`: create with form field `name`.
- `POST /admin/chassis/brands/{id}/update`: rename with `name`.
- `POST /admin/chassis/brands/{id}/disable` and `/enable`: change activation.
- `GET /admin/chassis/models`: all models and their parent brand/status, with
  create, rename, enable/disable forms.
- `POST /admin/chassis/models`: create with `brand_id` and `name`.
- `POST /admin/chassis/models/{id}/update`: rename with `name` only.
- `POST /admin/chassis/models/{id}/disable` and `/enable`: change activation.

All mutations use application/x-www-form-urlencoded forms, existing admin
allowlist authorization, and the existing CSRF field/cookie mechanism. Bodies
are limited to 8 KiB before CSRF parsing. Success returns a 303 redirect to the
corresponding listing; validation failures render a useful message (400),
duplicates return 409 with a human-readable message, and unknown UUID entries
return 404. Malformed IDs return 400. Internal failures return generic 500.

Names are trimmed and must contain 1–100 Unicode characters without controls.
`Custom` (case-insensitive) is reserved for the synthetic client option and is
rejected. PostgreSQL's existing case-insensitive unique indexes remain the final
uniqueness authority, including concurrent writes. Model names are unique only
within a brand. Index violations are translated to a domain duplicate error;
PostgreSQL details are never rendered.

A model's brand_id is immutable during editing for this POC; posting brand_id
on its rename route is rejected. New models require an active parent brand.
The repository locks the selected active parent with FOR SHARE and inserts in
one SQL statement, preventing a concurrent disable from invalidating that check.
No catalog row is deleted through these endpoints.

Authenticated selection API:

```text
GET /api/v1/chassis/brands
GET /api/v1/chassis/brands/{brand_id}/models
```

Responses are JSON arrays of only `{"id":"uuid","name":"Name"}` objects.
Empty results are `[]`. Unauthenticated calls return 401. The model-list endpoint
returns 404 for unknown or inactive brands; invalid UUIDs return 400. Unexpected
errors return generic 500. Existing exact-origin credentialed CORS still applies.
There are no client mutation endpoints.

Client SQL queries filter active values. Models require both their own and their
parent brand's activation flags to be true. Disabling a brand does not change its
models' flags; re-enabling it restores visibility of its active models. Disabled
records remain visible in admin lists and available through `Service.GetModel`
and `Repository.GetModel`, which return ModelState with historical names and
independent model/brand activation flags. Unknown model IDs return ErrNotFound.
These lookups prepare setup validation without implementing setup CRUD.

To create **Yokomo → RD2.0** manually:

1. Set DATABASE_URL, Google/session configuration and ADMIN_EMAILS as documented
   above, then start PostgreSQL, apply migrations and run the backend.
2. Open `http://localhost:8080/auth/google` and sign in with an allowlisted account.
3. Open `http://localhost:8080/admin`, then click **Brands**.
4. Enter `Yokomo` in **Brand name** and click **Create brand**.
5. Click **Models**, choose `Yokomo` in **Brand**, enter `RD2.0` in **Model name**,
   and click **Create model**.
6. Verify both entries show Active. In the same signed-in browser, open
   `/api/v1/chassis/brands`, copy Yokomo's ID, and open
   `/api/v1/chassis/brands/{that-id}/models` to see RD2.0.

The client can later display “Custom / not listed” and submit a NULL chassis
model reference; no Custom database record is needed. No setup, friendship,
search, statistics or admin users-list functionality was added in Phase 5.

Catalog tests use disposable PostgreSQL with automatically applied migrations,
real sessions and a fake Google provider. Coverage includes case-insensitive
uniqueness, rename, activation flags, immutable parent brands, stored historical
records, authenticated JSON selection, CSRF and admin access. Run
`go test ./...` and `go test -race ./...` with Docker running.

## Phase 6 owner setups

`internal/setups` contains explicit schema-v1 domain types, validation/service
logic, a pgx repository, and JSON handlers. It uses the existing setups table;
there are no new migrations, dependencies, or catalog records.

All five routes require the existing application session:

```text
POST   /api/v1/setups          -> 201, setup and Location header
GET    /api/v1/setups/{id}     -> 200, setup
PATCH  /api/v1/setups/{id}     -> 200, updated setup
DELETE /api/v1/setups/{id}     -> 204
GET    /api/v1/me/setups       -> 200, array (newest created first)
```

Responses include id, chassis_model_id, title, visibility, data, notes,
schema_version, created_at and updated_at. Owner ID is internal and never
accepted from clients. Phase 6 used owner-only reads; Phase 7 below extends read access by visibility.
Updates and deletes remain owner-scoped.

DataV1 has optional Suspension, Shocks and Electronics sections. Suspension has
optional Front/Rear AxleSuspension values with camber_deg, caster_deg, toe_deg
and link_lengths (name/length_mm). Shocks has optional Front/Rear Shock values
with manufacturer, model, Spring (manufacturer/color), and oil_cst. Electronics
has motor, esc, servo, gyro and radio strings. Optional numeric values use
*float64 with omitempty: an explicit zero survives JSON and JSONB round trips,
while omitted/null numeric values remain absent. Link length is required and
positive when a link is supplied; supplied oil must also be positive. Angles
have no narrow domain limits.

Create requires a nonempty trimmed title (maximum 150 Unicode characters), an
explicit visibility (public/friends/private), and an object-valued data document.
Notes are optional free-form text (maximum 10,000 characters). Technical strings
are trimmed and limited to 200 characters; an axle may contain up to 100 links.
Requests require application/json, have a 256 KiB body limit, and reject unknown
fields, malformed JSON and multiple JSON values. Validation returns 400;
unexpected failures return generic 500 without database details.

The backend inserts schema_version=1 and never allows clients to set it.
PATCH replaces only supplied top-level fields. Small explicit presence-bearing
types distinguish omission from null: null clears chassis_model_id or notes;
null title, visibility or data is invalid. Supplied data replaces the complete
document. SQL explicitly sets updated_at=now(); an optimistic timestamp check
returns 409 for a concurrent change, so clients can reload and retry without
silently erasing another partial update. Schema/owner fields cannot be patched.

Null chassis is allowed without a Custom row. New non-null selections reuse
the chassis service and require an existing active model under an active brand.
An unchanged historical chassis reference is retained and does not need to be
active on PATCH. It remains readable after disabling the model or brand;
changing to a different inactive chassis is rejected, and clearing is allowed.

To test **Yokomo → RD2.0** manually, start the backend as documented above and
ensure both catalog entries are active. Sign in at
`http://localhost:8080/auth/google`, then open
`http://localhost:8080/api/v1/me`. In that page's browser developer console,
run the following statements in order. They use your existing HttpOnly session
cookie without reading it or storing tokens:

```javascript
async function call(path, method = 'GET', body) {
  const response = await fetch('/api/v1' + path, {
    method, credentials: 'include',
    headers: body === undefined ? {} : {'Content-Type': 'application/json'},
    body: body === undefined ? undefined : JSON.stringify(body)
  });
  if (!response.ok) throw new Error(await response.text());
  return response.status === 204 ? undefined : response.json();
}

const brands = await call('/chassis/brands');
const yokomo = brands.find(b => b.name === 'Yokomo');
if (!yokomo) throw new Error('Create or enable Yokomo in the admin catalog');
const models = await call(`/chassis/brands/${yokomo.id}/models`);
const rd20 = models.find(m => m.name === 'RD2.0');
if (!rd20) throw new Error('Create or enable RD2.0 in the admin catalog');

// Create a private setup using the catalog model.
const setup = await call('/setups', 'POST', {
  title: 'RD2.0 carpet setup', chassis_model_id: rd20.id,
  visibility: 'private', notes: 'Initial setup',
  data: {suspension: {rear: {toe_deg: 0}}}
});
console.log(setup);

// Read, patch only the title, list your setups, then delete.
console.log(await call(`/setups/${setup.id}`));
console.log(await call(`/setups/${setup.id}`, 'PATCH', {title: 'RD2.0 revised title'}));
console.log(await call('/me/setups'));
await call(`/setups/${setup.id}`, 'DELETE');
// A subsequent GET of this ID returns 404.
```

Repository and HTTP tests use disposable real PostgreSQL databases with
automatically applied migrations and fake Google identities. They cover typed
JSONB persistence, numeric zero, schema ownership, strict input validation,
partial updates, ownership, catalog activation/history, listing and deletion.
Run `go test ./...` and `go test -race ./...` with Docker available.

## Phase 7 setup visibility

All setup routes still require authentication. Public means visible to any
signed-in application user; no anonymous access or public search was added.
The central read policy is `internal/setups/visibility.go`: owners may view
public/friends/private, accepted friends public/friends, and unrelated or pending
friends public only. Unknown visibility fails closed.

`GET /api/v1/setups/{id}` fetches the setup and applies that policy. Friendship
is queried only when it could change the decision. Missing/inaccessible setups
return identical 404 responses. `GET /api/v1/users/{user_id}/setups` checks the
caller/target relationship once, then passes the policy's allowed visibility
values to an explicit SQL `visibility=ANY(...)` filter. Rows that are private
or otherwise inaccessible are never loaded or decoded. Ordering is
created_at DESC, id DESC. Empty results and unknown valid user IDs return `[]`;
malformed user IDs return 400. No pagination was added.

`internal/friendships/repository.go` implements only an accepted-pair lookup.
It queries the unordered UUID pair with LEAST/GREATEST and status='accepted',
matching the existing unique expression index. No friendship management routes
exist yet. Relationships are checked afresh on requests; no cache delays
acceptance or removal. Listing uses at most one friendship lookup and one setup
query regardless of list size. `/me/setups` needs no friendship lookup.

PATCH explicitly reads through GetOwned, and update/delete SQL still constrains
both ID and owner ID. Broader read permission never grants mutation permission.
Creation still takes ownership from the application session. There are no new
migrations, configuration variables or dependencies.

Manual two-user test (development database only):

1. Sign in as A in a normal browser and B in an incognito/separate browser.
   Open `http://localhost:8080/api/v1/me` in each and record their IDs.
2. In A's developer console, define the `call` function from Phase 6 above and
   create one setup of each visibility:

   ```javascript
   const a = await call('/me');
   const examples = [];
   for (const visibility of ['public', 'friends', 'private']) {
     examples.push(await call('/setups', 'POST', {
       title: `A ${visibility}`, visibility, chassis_model_id: null, data: {}
     }));
   }
   console.log(JSON.stringify({owner: a.id, setups: examples.map(s => ({id: s.id, visibility: s.visibility}))}));
   ```

3. Copy A's ID and the three setup IDs. In B's console define `call`, then run
   `await call('/users/A_UUID/setups')`, substituting A's UUID. With no accepted
   friendship, only A's public setups appear. Detail GET of the public ID returns
   200; friends/private IDs return 404.
4. From backend/, open the local database:

   ```bash
   docker compose exec postgres psql -U rc_setup_dev -d rc_setup_hub
   ```

   Substitute the real IDs in these psql commands. This prepares test data
   directly without implementing Phase 8 endpoints:

   ```sql
   \set a 'A_UUID'
   \set b 'B_UUID'
   INSERT INTO friendships(requester_id,addressee_id,status)
   VALUES(:'a'::uuid,:'b'::uuid,'accepted') ON CONFLICT DO NOTHING;
   UPDATE friendships SET status='accepted',updated_at=now()
   WHERE LEAST(requester_id,addressee_id)=LEAST(:'a'::uuid,:'b'::uuid)
     AND GREATEST(requester_id,addressee_id)=GREATEST(:'a'::uuid,:'b'::uuid);
   ```

5. Repeat B's list: public + friends are now returned, private is absent.
   Detail GET of the friends ID is 200; private remains 404.
6. In B's console, run both requests for each of A's three setup IDs:

   ```javascript
   await call('/setups/SETUP_UUID', 'PATCH', {title: 'Forbidden'});
   await call('/setups/SETUP_UUID', 'DELETE');
   ```

   Each fails with 404, including public/friends setups B can read. A can still
   edit/delete them using the same requests from A's browser.

Tests use real migrated PostgreSQL containers and application sessions with fake
Google identities. They cover the complete visibility matrix, both request
orientations, pending/accepted/removed states, filtered newest-first listings,
identical missing/private errors, authentication and owner-only write regressions.
