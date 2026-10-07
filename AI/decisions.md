# Accepted decisions

This is a compact decision log. Codex must not silently contradict it.

## D001 — PostgreSQL, not MongoDB

Status: accepted.

Reason:
The application has relational ownership, friendship, catalog, permission, and search concerns. PostgreSQL also provides JSONB for flexible setup details.

## D002 — Hybrid relational + JSONB setup model

Status: accepted.

Normalized:
- users;
- friendships;
- chassis brand/model catalog;
- setup owner;
- setup visibility.

JSONB:
- suspension;
- shocks;
- electronics.

The JSONB document is represented by explicit Go structs.

## D003 — `schema_version` is mandatory

Status: accepted.

Every setup has an integer `schema_version`. Initial writes use version `1`.

## D004 — Chassis catalog is admin-managed

Status: accepted.

Only admins can create/change brands and models.

Regular users choose existing active values.

There is no real `Custom` brand/model database row.

`setups.chassis_model_id = NULL` means custom, unlisted, or unspecified. The POC does not distinguish those states.

## D005 — No discipline field in POC

Status: accepted.

Initial users are from a local RC drift audience. Do not add a discipline/category field until real requirements justify it.

## D006 — Admin panel is part of Go backend

Status: accepted.

Admin UI is server-rendered by the backend.

Do not create a separate admin SPA.

## D007 — Admin authorization comes from environment

Status: accepted.

Admin Google emails are provided by `ADMIN_EMAILS`.

Do not add `is_admin`, roles, or permission tables for POC.

## D008 — User client is separate

Status: accepted.

The user-facing client is a separate React application in its own repository
and calls the Go backend over exact credentialed CORS. No React/frontend code or
generated frontend types belong in the backend. This frontend direction is now
selected; repository locations need not be related.

## D009 — Simplicity beats speculative flexibility

Status: accepted.

Do not add extra tables/fields for custom chassis names, disciplines, likes, comments, version history, etc. until required.


## D010 — Committed frontend OpenAPI contract

Status: accepted.

Backend api/openapi.yaml (backend/api/openapi.yaml from the project root) is the
machine-readable OpenAPI 3.1 frontend API contract. The backend embeds and serves
it verbatim at GET /openapi.yaml. The separate frontend copies that file or
obtains it from a running backend, commits its own api/openapi.yaml, and generates
TypeScript types from its committed copy. No runtime reflection or backend
handler generation is required. Direct profile lookup uses safe PublicProfile;
full Setup includes owner application UUID and historical chassis names without
owner email/provider identity. Read-only catalog display includes inactive rows;
new selection remains active-only.

## D011 — Trusted frontend redirect and backend cookie session

Status: accepted.

APP_URL is required for normal backend startup, parsed/validated once as an
absolute HTTP(S) URL without credentials. Successful Google state/nonce/identity
verification and user upsert create the backend HttpOnly session cookie, then
redirect with 303 to configured APP_URL. Request redirect parameters are ignored.
APP_URL and ALLOWED_ORIGINS are separate settings. Frontend requests use
credentials: "include"; frontend JavaScript never receives or stores Google or
backend auth tokens/session handles. No JWT/localStorage auth is introduced.
Existing cookie lifetime/logout, Secure/SameSite and exact CORS behavior remain.


## D012 — Backend repository coordinates combined deployments

Status: accepted.

The backend repository is the sole GitHub Actions coordinator for staging and production. Both repositories keep independent CI, but deployment credentials and the combined deploy workflow live only here.

Every deployment is a combined release identified by explicit backend and frontend commit SHAs and containing the Go server, migrator and environment-specific frontend build. One-sided features still select a compatible SHA from the unchanged repository.

Staging and production are separate builds. Production deployment is manually dispatched in V1 and both submitted SHAs must be reachable from their respective `main` histories.
