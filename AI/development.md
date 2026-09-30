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

Expected configuration will include at least:

```text
APP_ENV
HTTP_ADDR
DATABASE_URL

GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
GOOGLE_REDIRECT_URL

SESSION_SECRET

ADMIN_EMAILS
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
