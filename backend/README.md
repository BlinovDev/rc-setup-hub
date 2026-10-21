# Backend local setup

Use the Go version in [go.mod](go.mod), Docker and the supported Ubuntu 24.04 native
photo decoder. [Provisioning script](../scripts/provision-photo-decoder.sh) pins
ImageMagick/libheif/libde265 and requires prlimit. Provision only an authorized local
runtime/CI runner; it does not authorize changes to a live deployment host. Native
macOS alone does not provide the supported Linux decoder/sandbox prerequisites.

## Start locally

Create a Google OAuth Web application client with the exact local callback
`http://localhost:8080/auth/google/callback`; configure consent/testing users as
needed. Substitute your own credentials below, never commit them. Run from backend/:

```sh
docker compose up -d --wait
export DATABASE_URL='postgres://rc_setup_dev:rc_setup_dev@127.0.0.1:5432/rc_setup_hub?sslmode=disable'
export HTTP_ADDR=':8080'
export GOOGLE_CLIENT_ID='YOUR_CLIENT_ID.apps.googleusercontent.com'
export GOOGLE_CLIENT_SECRET='YOUR_CLIENT_SECRET'
export GOOGLE_REDIRECT_URL='http://localhost:8080/auth/google/callback'
export SESSION_SECRET="$(openssl rand -base64 32)"
export SESSION_COOKIE_SECURE='false'
export SESSION_SAME_SITE='lax'
export APP_URL='http://localhost:5173'
export ALLOWED_ORIGINS='http://localhost:5173'
export MEDIA_DIR="$HOME/.local/share/rc-setup-hub/media"
# Optional, plain signed-in Google email(s), comma-separated; empty grants no admin.
export ADMIN_EMAILS=''
go run ./cmd/migrate up
go run ./cmd/server
```

The development-only PostgreSQL 17 credentials above are from
[compose](docker-compose.yml). It binds localhost; its named volume persists across
`docker compose down`. Do not use development credentials or share data with staging
or production. MEDIA_DIR must be a dedicated private absolute path outside public
static/release roots; startup creates it mode 0700 and performs actual HEIC conversion.

SESSION_SECRET is base64 of at least 32 random bytes; generate once per running
configuration, not on each request. Use localhost on both client/backend origins.
Vite runs separately at port 5173; exact CORS and SameSite are independent of APP_URL.
Production HTTPS and genuinely cross-site cookie requirements are in
[security](../AI/architecture.md#authentication-and-security).

## Smoke and migrations

```sh
curl -i http://localhost:8080/health
curl --fail http://localhost:8080/openapi.yaml -o /tmp/rc-setup-hub-openapi.yaml
```

Navigate the browser to `/auth/google`; success returns 303 to configured APP_URL.
The frontend resolves `/api/v1/me` with credentials. Sign out must revoke the session.
Allowlisted users can visit `/admin`; missing session is 401, non-admin is 403.

Health checks PostgreSQL connectivity, not schema readiness. Server startup does not
apply migrations. `go run ./cmd/migrate up` applies outstanding embedded goose SQL;
only DATABASE_URL is required for migrations. `go run ./cmd/migrate down` is destructive:
inspect the actual migration, intentionally authorize data loss and archive coherent
DB/media before rollback. Current files are in [migrations](migrations); do not copy
an incomplete migration inventory into this README. No production rollback is automatic.

## Further context

[API](../AI/api.md) owns endpoint/error usage; [database](../AI/database.md) owns
persistence/cleanup constraints; [testing](../AI/testing.md) owns automated checks.
Tests use disposable migrated PostgreSQL via Docker, not the development database,
and fake Google/OIDC providers. Missing Docker fails full tests; -short is explicit
unit-only evidence. Missing native codecs never justify silently skipping photo tests.

[Deployment](../AI/deployment.md) and the [host runbook](../scripts/server/README.md)
own paired releases, runtime sandbox, private media and coherent backups. Starting a
new binary on a live unprovisioned host is not a local smoke test. Earlier phase/API
examples and verification reports remain in [Git history](../AI/archive/README.md).
