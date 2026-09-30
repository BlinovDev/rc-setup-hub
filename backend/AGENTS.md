# Backend-specific Codex instructions

These instructions apply to files under `backend/`.

Read the root `AGENTS.md` and the relevant `AI/*.md` files first.

## Technical direction

- Language: Go.
- HTTP router: `chi`.
- PostgreSQL driver/pool: `pgx/v5`.
- Migrations: SQL migrations; use `goose` unless an accepted decision changes this.
- Admin HTML: Go `html/template`; do not introduce React or another SPA framework for `/admin`.
- API responses: JSON.
- IDs: UUID.
- Times: UTC in the application and `TIMESTAMPTZ` in PostgreSQL.
- Configuration: environment variables.
- Authentication: Google only; backend-issued authenticated session; never store auth tokens in browser localStorage.
- CORS: exact configured client origins, never `*` together with credentials.

## Package boundaries

Target layout:

```text
cmd/server              application wiring only
internal/config         environment/config loading
internal/database       connection pool, migrations/bootstrap helpers
internal/auth           Google login/session logic
internal/users          user domain, service, repository, HTTP endpoints
internal/friendships    friend request/domain logic
internal/chassis        admin catalog + public read endpoints
internal/setups         setup domain, JSON schema, visibility, search
internal/admin          admin dashboard and server-rendered handlers
internal/web            shared HTTP middleware/helpers
migrations              ordered SQL migrations
templates/admin         server-rendered admin templates
```

Avoid generic `utils`, `helpers`, or `common` packages unless there is a concrete shared responsibility.

## Handler rule

HTTP handler:
1. decode and validate transport input;
2. call a service/use-case;
3. map the result/error to HTTP;
4. encode response.

Handlers must not contain raw SQL.

## Database rule

Repository methods own SQL. Keep queries explicit and readable. Do not introduce an ORM for the POC.

## Setup JSON rule

`setups.data` is JSONB physically, but in Go it must be represented by explicit structs. Optional numeric fields must use pointers (or equivalent nullable types) so that an omitted value is distinguishable from a valid zero such as `toe_deg = 0`.

## Tests

Prefer table-driven unit tests for pure logic. Repository behavior must be tested against real PostgreSQL, not only SQL mocks, because the project relies on JSONB, constraints, indexes, and PostgreSQL-specific behavior.
