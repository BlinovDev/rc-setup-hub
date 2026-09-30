# Backend — Phase 3

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
