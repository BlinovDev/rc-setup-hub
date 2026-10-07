# Deployment

GitHub Actions CI runs independently in both repositories. Combined application deployment is coordinated only from the backend repository `BlinovDev/rc-setup-hub`.

## Runtime

Production and staging share one Hetzner VPS but have isolated Go processes, environment files, PostgreSQL containers/databases/credentials/volumes, release stores, frontend roots and backend ports.

- Production: `https://international-drift-hub.com`, backend loopback `127.0.0.1:18080`.
- Staging: `https://staging.international-drift-hub.com`, backend loopback `127.0.0.1:18081`.
- nginx serves Vite static output and proxies `/api`, `/auth`, `/admin`, `/health` and `/openapi.yaml` to Go.
- PostgreSQL is Docker Compose-managed; application processes are host systemd services.
- Application secrets live only on the server. GitHub Actions must not receive database, OAuth or session secrets.

## Release model

Every deployment is one combined archive containing:
- Linux ARM64 Go server;
- Linux ARM64 migration executable;
- environment-specific Vite `dist/`;
- manifest identifying one backend SHA and one frontend SHA.

Backend and frontend SHAs are independent full 40-character Git commit IDs. A one-repository change is still deployed as a pair with an explicitly selected compatible SHA from the other repository.

The backend repository is the sole deployment coordinator. `.github/workflows/deploy.yml` is manually dispatched from trusted workflow code and resolves the two selected refs once before validation/build.

Production and staging bundles are different because the frontend embeds a different `VITE_API_URL` and the manifest records the environment. Do not promote the staging tar byte-for-byte to production.

## Validation

Before packaging, the coordinator:
- validates backend formatting, dependencies, tests and build;
- verifies frontend OpenAPI equals the selected backend OpenAPI;
- regenerates frontend API types and requires a clean diff;
- runs frontend format/lint/typecheck/tests;
- builds the frontend for the selected origin;
- cross-compiles server and migrator for Linux ARM64.

The server revalidates manifest checksums, required files, ARM64 architecture and expected frontend origin.

Production additionally rejects either SHA unless it is reachable from the corresponding repository's `main` history.

## GitHub environments

Create `staging` and `production` in the backend repository.

Each environment needs secret:
- `DEPLOY_SSH_PRIVATE_KEY` — its own forced-command deployment key.

Each environment needs variables:
- `DEPLOY_HOST=37.27.218.221`
- `DEPLOY_PORT=22`
- `DEPLOY_USER=deploy`
- `DEPLOY_ENV=staging` or `production`
- `DEPLOY_KNOWN_HOSTS` — administrator-verified complete SSH host identity line.

Never use `StrictHostKeyChecking=no` or runtime-unverified `ssh-keyscan`.

Restrict production environment deployment branches to `main`. Configure required reviewers if the GitHub plan supports them. Deployment remains manual in V1 even when CI passes.

## Server SSH protocol

The two deployment public keys are forced-command keys. Staging credentials cannot invoke production and production credentials cannot invoke staging. They do not provide arbitrary shell, SCP, SFTP or rsync.

The workflow streams the release tar over stdin using:
- `upload ENV BACKEND_SHA FRONTEND_SHA`;
- then `deploy ENV BACKEND_SHA FRONTEND_SHA`;
- then `status ENV`.

The server owns database backup, migration, candidate health verification, activation and public smoke checks. Actions must not run migrations directly or read server environment files.

There is no automatic application/database rollback. A failed deployment can leave migrations applied or, after activation begins, a partially switched application. Do not cancel an in-flight deployment; use environment concurrency with `cancel-in-progress: false`. Recovery is an administrator operation.

## Staging prerequisites

Before first staging deployment:
1. create DNS `staging.international-drift-hub.com -> 37.27.218.221`;
2. issue/enable staging TLS using the prepared root helper;
3. configure staging Google OAuth client/callback and replace server placeholders;
4. configure backend repository GitHub environments/keys/known-host identity.

Until these are complete, staging deployment is expected to fail the server readiness checks.

## Production release procedure

After QA:
1. merge the required backend/frontend changes to their respective `main` branches;
2. resolve the actual resulting main SHAs (important for squash/rebase merges);
3. manually dispatch the backend coordinator for `production` with those refs/SHAs;
4. approve the GitHub production environment if reviewer protection is configured;
5. require the workflow and server smoke checks to succeed.

Do not deploy an unmerged feature SHA to production.
