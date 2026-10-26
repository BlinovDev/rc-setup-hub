# Backend development

Read only relevant [architecture/security](architecture.md), [API](api.md) and
[database](database.md) sections for the task. Use [local setup](../backend/README.md)
for startup commands; do not duplicate them in feature reports.

## Coding rules

- Prefer table-driven unit tests for pure logic.
- Standard Go formatting; context.Context at request/database boundaries; explicit
  dependency injection, no global mutable state/package database handle.
- Typed visibility/friendship constants and optional numeric pointers; explicit
  transport DTOs when useful. Document public types/functions where purpose is unclear.
- Domain/service invariants stay out of SQL handlers; repositories own explicit SQL.
  Validate at both transport boundary and domain layer; no speculative interfaces.
- JSON create/PATCH reject malformed/trailing/unknown fields and enforce body limits.
  Central logging/recovery; typed errors map safely to HTTP, no internals to clients.
- PostgreSQL only, pgx, goose SQL migrations, UUID IDs, TIMESTAMPTZ/UTC. Inspect real
  migrations rather than maintaining a second schema listing in README.
- Schema-v1 technical data remains structured. Incompatible JSON evolution requires
  a new version, documented read/migration strategy and old/new regression tests
  before changing write defaults; never silently reinterpret existing documents.
- Keep scope to approved Issue criteria. Historical phases are not active instructions.
  Docs change only for enduring rules/contracts/commands, per [workflow](workflow.md).

## Environment contract

Use environment configuration: HTTP_ADDR, DATABASE_URL, MEDIA_DIR, GOOGLE_CLIENT_ID,
GOOGLE_CLIENT_SECRET, GOOGLE_REDIRECT_URL, SESSION_SECRET, SESSION_COOKIE_SECURE,
SESSION_SAME_SITE, ADMIN_EMAILS, APP_URL and ALLOWED_ORIGINS. Source validation is in
[config](../backend/internal/config). No secrets in Git/logs. Local dotenv convenience
must not become a production dependency; production runs with environment values.

APP_URL is required for the server, separately from CORS. Migration commands require
only DATABASE_URL; server startup does not run migrations. Local ports/Google callback
and secret generation are in [local setup](../backend/README.md); production cookie,
origin, CSRF and token rules remain in [security](architecture.md#authentication-and-security).

## Contract delivery and checks

Backend owns [OpenAPI](../backend/api/openapi.yaml); update it with changed handlers,
DTOs/error mappings and tests. FE copies exact bytes and runs its own
`npm run api:generate` into `src/api/generated/schema.ts`; never put client code or
an invented generated path in this repository. Check [API guidance](api.md).

Run formatting and [relevant checks](testing.md), including real PostgreSQL for
persistence behavior; short tests are not complete integration evidence. No merge,
live host changes or deployment without explicit authorization and existing gates.
