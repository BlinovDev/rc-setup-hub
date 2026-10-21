# Backend architecture and security

One Go service owns JSON API, Google OAuth, sessions, server-rendered admin, domain
services and PostgreSQL access. The React/Vite client is a separate repository and
deployable build, served by nginx in production. Do not split the backend into
microservices or add an ORM. Accepted rationale remains in [decisions](decisions.md).

## Package boundaries

`backend/cmd/server` is wiring only; `internal/{auth,users,chassis,setups,friendships,
admin,config,database,photos,web}` own their responsibilities. Domains contain typed
models, explicit repositories, services and handlers. Handlers validate transport,
call use cases and map errors; no raw SQL in handlers. Repositories own pgx SQL.
Use chi, pgx/pgxpool, goose migrations, Google OAuth2/OIDC and UUIDs; dependency
versions live in [go.mod](../backend/go.mod). Avoid generic utility layers/interfaces
without a real consumer. Admin HTML uses escaped, buffered html/template.

Relational identities/ownership/catalog/friendships plus typed schema-v1 JSONB
remain in PostgreSQL. No arbitrary map-based technical data. See [database](database.md)
for atomicity, keys/indexes, photo receipts/outbox and migration/rollback constraints.

## Authentication and security

- Google only. Browser navigation starts `/auth/google`; OIDC callback upserts by
  stable subject and returns 303 to trusted APP_URL, never client-controlled redirect
  parameters. Verify signature, issuer, audience, expiry, nonce, verified email,
  authorized-party and supplied access-token hash; exchange uses S256 PKCE.
- State is random, browser-bound, signed HttpOnly, expires in ten minutes and is
  atomically single-use, including denial/exchange failure. No provider token in logs,
  redirects, cookies or frontend storage/state. Google scopes are openid/email/profile.
- Gorilla securecookie signs an opaque random host-only rc_session handle. User ID
  and 24-hour expiry stay in bounded memory; login rotates, logout revokes. Restarts
  invalidate sessions and flows. No session DB table, persistent multi-replica design
  or client JWT is introduced.
- Production HTTPS requires Secure cookies. Local same-host ports use SameSite=Lax
  and Secure=false; genuinely cross-site HTTPS needs SameSite=None plus Secure and
  remains subject to browser cookie policy. Exact credentialed ALLOWED_ORIGINS is
  independent of APP_URL; never use wildcard CORS or silently trust additional origins.
- APP_URL is required at normal startup: absolute HTTP(S), valid host/port, no embedded
  credentials; trim/normalize host, preserve configured path/query/fragment. Invalid
  configuration fails startup; per-request return_to/redirect_uri is ignored.
- API mutations retain origin checks. Session identity supplies owner IDs; never trust
  client ownership. Visibility filters and owner checks run before protected data
  decoding; detail rechecks owner/visibility and fails closed if it changes.
- Admin authentication (401) and allowlist authorization (403) are distinct. ADMIN_EMAILS
  is trimmed/lowercased, plain valid emails only, empty grants nobody; invalid syntax
  fails configuration. No role table. Gorilla CSRF protects the entire admin group,
  including future mutations, with separate signed HttpOnly `/admin` cookie, SameSite=Lax,
  Secure setting and strict HTTPS Origin/Referer checks. Only library-generated
  CSRFField is trusted HTML; user strings stay escaped. Forms accept hidden token/header.
- Public profiles expose safe application UUID/nickname/avatar only. Owner email and
  provider identity never appear in public setup/profile/search projections.
- JSON responses and admin HTML use no-store; unexpected errors remain generic.
  Central request logging/panic recovery must not disclose credentials or raw DB errors.

## Private media boundary

Image signatures, not names/MIME, choose the pinned Ubuntu 24.04 native decoder.
Keep [product limits](product.md#optional-photos-and-framing), first-frame normalization,
metadata stripping, private runtime-owned storage and generated safe keys. No public
nginx media alias. Bounded resources/time/concurrency and actual HEIC startup probe
are mandatory; module/source implementations define exact current decoder limits.

Authorize before parsing/decoding, filter before file access and revalidate reads;
no-store/private/nosniff, no conditional cached 304 or exposed filesystem keys. Crops
use normalized full-image coordinates, immutable version publication and parent locks;
never rewrite the full image. See [database](database.md) for recovery and durable cleanup,
[deployment](deployment.md) for coherent DB+media snapshots and strict sandbox gates.

## HTTP and observability

The [OpenAPI contract](../backend/api/openapi.yaml) is embedded verbatim at unauthenticated
GET /openapi.yaml; no runtime reflection/handler generation. [API](api.md) owns transport
usage details. Setup lists join historical chassis and cover metadata without N+1;
active-only selectors do not erase historical display. Custom chassis is null.

Structured method/path/status/duration logs, redacted startup diagnostics and `/health`
are the minimum; no new metrics platform by assumption. Database connectivity failure
is fatal at startup. Health pings DB with a two-second timeout (200/503), not migration
state. SIGINT/SIGTERM allow ten-second HTTP shutdown, then close the pool. Use UTC.
