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

## Photo runtime provisioning and persistent storage (Issue #5)

This feature introduces a required host prerequisite. No live server was changed.
Supported/pinned baseline: Ubuntu 24.04 amd64/arm64, ImageMagick
8:6.9.12.98+dfsg1-5.2build2, libheif1/libheif-plugin-libde265 1.17.6-1ubuntu4.9,
libde265-0 1.0.15-1ubuntu0.2. The HEVC plugin is required even if identify advertises
HEIC. Run scripts/provision-photo-decoder.sh only after host-change authorization.
It fails on a different OS or unavailable package version instead of guessing.
Security/pin updates need review and rerun amd64/ARM64 gates. CI provisions these
pins and runs native ubuntu-24.04-arm conversion tests plus a CGO-disabled build.
The Go release stays CGO_ENABLED=0; native codecs are host executables/plugins,
not linked into the binary or presumed bundled.

Before release, verify the actual VPS OS/architecture and provision the baseline;
a different OS blocks deployment pending a reviewed compatible provisioning
change. As each environment's runtime user run the packaged server --check-media
with MEDIA_DIR: no DB/OAuth needed, actual HEIC conversion/writeability checked.
Normal startup also probes and reconciles before accepting requests. The host
helper must add this gate to candidate verification before deployment (an
administrator update is required; none was made in this task).

Use separate runtime-owned mode-0700 persistent directories, for example staging
MEDIA_DIR=/var/lib/international-drift-hub-staging/media and production
MEDIA_DIR=/var/lib/international-drift-hub/media. Never share path, user or bind
mount between environments. Add the appropriate path to systemd ReadWritePaths
when ProtectSystem is enabled. Keep outside code/releases/frontend roots; never
nginx alias/static-serve it. Proxy media through authenticated API routes.
Set nginx client_max_body_size 26m for /api and proxy_read_timeout 120s; provision
private request-body temporary storage. Go limits source 25 MiB+64 KiB overhead,
50 MP, two concurrent uploads, 45-second body reads, 30-second decoder wall time,
20-second CPU, 1 GiB address space, 256 MiB memory/256 MiB map/512 MiB disk cache
and 64 MiB output file size. Monitor free space/inodes and cleanup-error logs;
reserve headroom for two source files, outputs, bounded decoder scratch and backups.

Permission/disk/decoder failure returns safe errors, without usable-looking
metadata or retained raw originals as a feature. Fsynced processed directories
publish by atomic rename before metadata+receipt commit. Uncertain commit keeps
files for safe retry; unreferenced/temp work older than one hour is reclaimed only
after a successful DB inventory. Durable deletion outbox survives cascades,
failures/restarts and is retried on startup/every minute. Never clear it manually
to hide storage failures. Release archives contain no media.

### Coherent DB+media backup/restore and retention

Previous DB-only backups are insufficient. Before deploying, an authorized admin
must update the host helper/backup job for coherent snapshots; until then release
is blocked. Maintenance procedure: block API mutations/uploads at nginx, drain
requests, stop that environment's Go service (pauses cleanup), create PostgreSQL
17 pg_dump custom-format backup and matching media tar/filesystem snapshot. Store
both plus environment, BE/FE SHAs, schema/decoder versions and checksums under ONE
snapshot ID. Include photo metadata, receipts and deletion outbox. Exclude
abandoned .upload-* work. Restart/reopen only after BOTH backups succeeded; never
label half-failed or DB-only snapshots usable. Reconciliation on restart handles
queued deletions/aged orphans.

Restore into an isolated test environment first: stop service, verify checksums,
restore DB and media from the SAME snapshot without merging current files or
mixing environments, fix runtime ownership/0700, provision recorded decoders,
run --check-media, apply migration readiness checks and start. Verify every photo
record has full+thumbnail files, one cover for nonempty galleries, authenticated
reads and retained technical data. Missing files mean a failed restore, not silent
metadata deletion. Automated tests exercise actual pg_dump/pg_restore and media
copy using two disposable PostgreSQL 17 databases and separate directories;
source deletion must not affect the restored set. Live host/service/backup tooling
remains a separately authorized administrator QA gate and was not tested here.

Default retention: last seven successful daily coherent snapshots and all
pre-migration sets for 30 days, encrypted with backup credentials outside Git.
Prune DB/media/manifests together only after a newer verified set exists. Backups
are sensitive and must stay outside public/static roots. Run an isolated restore
drill before relying on recovery. Prior downloaded copies cannot be revoked.
