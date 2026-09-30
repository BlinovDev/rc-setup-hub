# Backend — Phase 2

## Local PostgreSQL

Run these commands from `backend/` with Docker running:

```sh
docker compose up -d --wait
export DATABASE_URL='postgres://rc_setup_dev:rc_setup_dev@127.0.0.1:5432/rc_setup_hub?sslmode=disable'
go run ./cmd/migrate up
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
Email normalization and active-catalog selection rules belong to the later
user/catalog/setup services. This phase enforces database uniqueness, foreign
keys, NOT NULL and check constraints without adding domain services or triggers.
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

Docker unavailability fails the integration suite. To explicitly run only unit
and HTTP tests without Docker, use `go test -short ./...`.
