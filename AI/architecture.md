# Architecture

## High-level topology

```text
Browser / future client app
        |
        | JSON API over HTTPS
        | CORS for configured client origins
        v
+-----------------------------+
| Go backend                  |
|                             |
| /api/v1/*     JSON API      |
| /auth/*       Google auth   |
| /admin/*      HTML admin    |
+-------------+---------------+
              |
              | pgx
              v
        PostgreSQL
```

## Backend is one deployable service

Do not split the POC into microservices.

The same Go process owns:
- API routing;
- authentication;
- session handling;
- admin routes/templates;
- application services;
- PostgreSQL access.

## Client is a separate deployable application

The client is not served by the Go backend.

The exact client technology is intentionally not part of the backend contract yet. React web/PWA is the preferred POC direction because it minimizes build/deployment complexity, but a native client must be able to consume the same API.

## Suggested Go libraries

Keep dependencies small:

- router: `github.com/go-chi/chi/v5`;
- PostgreSQL: `github.com/jackc/pgx/v5`;
- SQL migrations: `github.com/pressly/goose/v3`;
- UUID: use a small UUID package if needed by application code, otherwise let PostgreSQL generate UUIDs;
- Google OAuth/OIDC: use official/standard Go OAuth/OIDC-compatible libraries; exact package can be selected during the auth milestone.

Do not add an ORM.

## Authentication direction

Google is the only identity provider for the POC.

Preferred browser flow:

1. User starts login through the backend.
2. Backend performs Google OAuth/OIDC flow.
3. Backend verifies the Google identity.
4. Backend upserts the application user by stable Google subject (`sub`).
5. Backend creates its own authenticated session.
6. Browser receives a secure `HttpOnly` session cookie.
7. The separate client calls the API with credentials enabled.

Do not treat Google email as the external identity key. Store the stable Google subject separately.

The session implementation must not require a database table unless there is a clear reason. A signed/encrypted server-managed cookie is acceptable for the POC.

## Admin authorization

Authentication and admin authorization are separate concerns.

A user is an admin only when the authenticated Google/user email is present in:

```text
ADMIN_EMAILS=admin1@example.com,admin2@example.com
```

No `role`, `is_admin`, or permissions table is required for the POC.

Admin middleware must reject authenticated non-admin users with HTTP 403.

## CORS

CORS applies to the JSON API used by the separate client.

Configuration must use explicit origins, for example:

```text
ALLOWED_ORIGINS=http://localhost:5173
```

Production can contain the deployed client origin.

When cookie credentials are used:
- enable credentials;
- never use wildcard `*` origin;
- permit only required methods and headers.

The backend admin panel is same-origin with the backend and does not depend on CORS.

## Package structure

```text
backend/
├── cmd/server/
├── internal/
│   ├── admin/
│   ├── auth/
│   ├── chassis/
│   ├── config/
│   ├── database/
│   ├── friendships/
│   ├── setups/
│   ├── users/
│   └── web/
├── migrations/
└── templates/admin/
```

Each domain package can contain its own:
- model/types;
- repository;
- service/use-case;
- handlers.

Do not create an abstraction layer solely to make the directory tree look clean.

## Error model

Domain/service errors should be explicit enough for handlers to map them to:
- 400 invalid input;
- 401 unauthenticated;
- 403 forbidden;
- 404 not found;
- 409 conflict;
- 500 unexpected server error.

Do not expose raw database errors to API clients.

## Observability for POC

Minimum:
- structured application logs;
- request method/path/status/duration;
- startup configuration summary without secrets;
- database connection failure is fatal at startup;
- `/health` endpoint.

No metrics platform is required for the first POC.
