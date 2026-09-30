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

The user-facing client is a separate application and calls the backend over CORS.

Frontend technology can be finalized later without changing the backend domain architecture.

## D009 — Simplicity beats speculative flexibility

Status: accepted.

Do not add extra tables/fields for custom chassis names, disciplines, likes, comments, version history, etc. until required.
