# Architecture

## High-level topology

```text
Browser / separate React app
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

React is developed as a separate application in its own repository; no frontend code or generated TypeScript belongs in the backend. The committed OpenAPI 3.1 contract at `backend/api/openapi.yaml` describes implemented API DTOs, authentication, setup schema v1 and pagination. The backend embeds and serves it verbatim at unauthenticated `GET /openapi.yaml`. The frontend keeps its own committed copy at api/openapi.yaml for reproducible TypeScript generation. Obtain it by copying backend/api/openapi.yaml or downloading GET /openapi.yaml; repository locations are independent. No runtime reflection or backend handler generation is used.

## Suggested Go libraries

Keep dependencies small:

- router: `github.com/go-chi/chi/v5`;
- PostgreSQL: `github.com/jackc/pgx/v5`;
- SQL migrations: `github.com/pressly/goose/v3`;
- UUID: use a small UUID package if needed by application code, otherwise let PostgreSQL generate UUIDs;
- Google OAuth/OIDC: golang.org/x/oauth2 with Google endpoints and github.com/coreos/go-oidc/v3 identity verification.

Do not add an ORM.

## Authentication direction

Google is the only identity provider for the POC.

Implemented browser flow:

1. User starts login through the backend.
2. Backend performs Google OAuth/OIDC flow.
3. Backend verifies the Google identity.
4. Backend upserts the application user by stable Google subject (`sub`).
5. Backend creates its own authenticated session.
6. Browser receives a secure `HttpOnly` session cookie.
7. Backend returns 303 to trusted configured `APP_URL`, with no authentication data in the URL.
8. The separate client calls `/api/v1/me` and the JSON API with `credentials: "include"`.

`APP_URL` is required, parsed at startup as an absolute HTTP(S) URL without credentials. Per-request redirect parameters are ignored. A configured landing-page path/query/fragment is preserved. `APP_URL` and the CORS allowlist are independent settings.

Do not treat Google email as the external identity key. Store the stable Google subject separately.

The session implementation uses Gorilla securecookie to sign an opaque random cookie handle. The user ID and 24-hour expiry remain in a bounded in-memory store; logout revokes the handle and login rotates it. HttpOnly host-only cookies never contain Google tokens. Restart invalidates sessions and pending OAuth flows. No session database table is needed. Local localhost frontend/backend ports are same-site and use SameSite=Lax with Secure=false. Production HTTPS uses Secure=true; genuinely cross-site frontend/API deployments require SameSite=None with Secure, subject to browser cookie policy.

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
APP_URL=http://localhost:5173
ALLOWED_ORIGINS=http://localhost:5173
GOOGLE_REDIRECT_URL=http://localhost:8080/auth/google/callback
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
├── api/openapi.yaml
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


## Direct frontend URLs

Authenticated GET /api/v1/users/{user_id} returns the safe public profile only.
Full Setup responses expose owner_id, allowing direct /setups/:setupId URLs to
resolve the owner without navigation state. They include nullable read-only
chassis metadata (model/brand UUIDs and names), joined without active filters so
historical disabled chassis can still display. NULL model yields chassis=null.
Selection endpoints remain active-only. Setup lists join metadata in one query.
Detail authorization still reads only owner/visibility metadata before protected
JSON decoding; the full query requires the same authorized owner/visibility,
failing closed on changes. Writes remain scoped to owner ID.

Frontend JavaScript never receives or stores Google/backend auth tokens or
session handles. Browser-managed HttpOnly cookies provide authentication;
credentialed requests and exact ALLOWED_ORIGINS remain required. APP_URL is a
separate required server setting, not an inferred CORS origin.
