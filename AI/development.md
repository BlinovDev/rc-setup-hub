# Development guide

## Main principle

Optimize for a small, understandable POC that can be changed quickly after real track users try it.

Do not prematurely build a generalized RC platform.

## Backend coding style

- Follow standard Go formatting and conventions.
- Use `context.Context` for request/database boundaries.
- Keep public types/functions documented when their purpose is not obvious.
- Prefer explicit dependencies passed through constructors.
- Avoid global mutable state.
- Avoid package-level database handles.
- Return errors with enough context for logs, without leaking internals to clients.
- Use typed constants for visibility and friendship statuses.
- Keep JSON transport DTOs separate from database rows when that improves clarity.
- Do not create interfaces before there is a consumer that benefits from abstraction.

## Configuration

Read configuration from environment.

Server configuration:

```text
HTTP_ADDR
DATABASE_URL

GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
GOOGLE_REDIRECT_URL

SESSION_SECRET
SESSION_COOKIE_SECURE
SESSION_SAME_SITE

ADMIN_EMAILS
APP_URL
ALLOWED_ORIGINS
```

No secret values belong in Git.

Local development may use a `.env` loader, but production must work with normal environment variables.

## Database

- PostgreSQL only.
- `pgx/v5`.
- Explicit SQL repositories.
- SQL migrations checked into Git.
- No ORM.
- Do not let handlers run SQL directly.
- Use `TIMESTAMPTZ`.
- Store times in UTC.

## HTTP

- Router: chi.
- JSON content type for API.
- Reasonable body size limits.
- Reject malformed JSON.
- Reject unknown JSON fields for create/update requests unless there is a strong compatibility reason not to.
- Central request logging and panic recovery middleware.
- Authentication middleware resolves the current application user.
- Authorization stays close to the resource/service logic.

## Security baseline

For the POC:
- secure HttpOnly authenticated session;
- `Secure` cookies in production;
- appropriate `SameSite` policy for the chosen app/api deployment;
- exact CORS allowlist;
- no wildcard credentialed CORS;
- CSRF protection on server-rendered admin form mutations;
- validate Google issuer/audience/token exchange correctly;
- admin email allowlist parsed from environment;
- never trust a client-supplied owner id;
- setup owner comes from the authenticated session;
- never expose raw SQL/database errors.

## Setup data evolution

Current schema version is:

```text
1
```

When the JSON shape changes incompatibly:
1. define a new version;
2. document the change;
3. decide whether old records are read through compatibility code or migrated;
4. add tests for both old and new behavior during migration;
5. only then make the new version the write default.

Do not silently reinterpret old JSON.

## Feature scope discipline

Before coding a requested feature, check `AI/roadmap.md`.

If the feature belongs to a later milestone, do not opportunistically implement it while working on an earlier milestone unless it is strictly necessary.

## Definition of done

For every change:
- `gofmt` has been run;
- tests for changed behavior pass;
- integration tests run when SQL behavior changes;
- no new secret/config value is undocumented;
- API/database contract docs are updated when they change.


## Separate frontend integration

Normal server startup requires APP_URL, an absolute HTTP(S) URL with host and
valid port, without URL credentials. It is parsed once, trims whitespace and
normalizes host casing; a trusted configured landing-page path/query/fragment is
preserved. Missing/malformed/relative values fail startup. Migration commands do
not require auth/frontend settings. No per-request return_to/redirect_uri is used.

Local configuration:

```text
APP_URL=http://localhost:5173
ALLOWED_ORIGINS=http://localhost:5173
GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback
SESSION_COOKIE_SECURE=false
SESSION_SAME_SITE=lax
```

Start login by browser navigation to backend /auth/google. Google returns to the
backend callback, which verifies state/nonce/identity, upserts the user, creates
its HttpOnly cookie and sends 303 to APP_URL. On the frontend origin call
http://localhost:8080/api/v1/me with credentials: "include". Use localhost on
both browser origins. Production HTTPS must enable Secure; same-site deployment
can retain Lax, genuinely cross-site requires None+Secure and browser cookie
support. APP_URL does not automatically populate ALLOWED_ORIGINS; exact
credentialed origins remain explicitly configured. No tokens in localStorage or
redirect URLs.

The committed OpenAPI 3.1 source is backend/api/openapi.yaml, embedded at
GET /openapi.yaml. Update it with every API contract change, comparing actual
handlers/DTOs/error mappings/tests instead of assuming older prose is correct.
The automated api package tests validate the specification and references using
libopenapi/libopenapi-validator, endpoint/schema/privacy coverage and
representative Go response DTOs/create/PATCH bodies. No external network is
needed by OpenAPI validation tests. Run go test ./... and go test -race ./...
with Docker running for the existing real PostgreSQL integration suites.

The user frontend is a separate React application in its own repository. Its
checkout may live anywhere. Obtain the contract in either of these ways:

1. Copy backend/api/openapi.yaml into the frontend repository's api/openapi.yaml.
2. Download the same YAML from a running backend at GET /openapi.yaml.

Commit that copy in the frontend repository for reproducible type generation.
Run these commands from the frontend repository (not the backend):

```sh
# If copied from the backend, generate from the frontend's committed contract:
npx openapi-typescript api/openapi.yaml -o src/generated/api.d.ts
# Or retrieve from a running backend, then commit the downloaded contract:
curl --fail http://localhost:8080/openapi.yaml -o api/openapi.yaml
npx openapi-typescript api/openapi.yaml -o src/generated/api.d.ts
```

Keep generated frontend types and application
code out of this backend repository. See backend/README.md for browser/CORS
verification and the complete local startup exports.


Frontend JavaScript never receives/stores Google tokens, backend auth tokens or
session handles; authentication is the browser-managed backend HttpOnly cookie.
For direct route reloads use GET /api/v1/users/{user_id} for a public profile and
full Setup.owner_id to resolve its owner. Setup.chassis provides historical
model/brand names without relying on active selection lists. Do not send
owner_id/chassis display fields in create or PATCH requests: they are read-only.
Keep contract tests for these projections and authorization-before-data-decode.
