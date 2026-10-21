# Host deployment helpers

These are administrator-installed, root-owned host files, **not** workflow payloads.
Workflows still only stream validated immutable releases through the existing forced
SSH `upload/deploy/status ENV BE_SHA FE_SHA` protocol. Never allow a deployment key
to replace these helpers or choose root command/filesystem arguments.

- `rc-hub-control.py` is the existing host controller, now tracked for review.
  Installed at `/usr/local/sbin/rc-hub-control`, root:root 0755.
- `staging_media.py` is installed at `/usr/local/lib/rc-hub/staging_media.py`,
  root:root 0644; its parent is root:root 0755.
- `staging-backup.sh` replaces only the staging timer entry point at
  `/srv/international-drift-hub/environments/staging/backup-postgres.sh`,
  root:root 0700. The existing backup unit/timer schedule is retained.

Install these together from a reviewed revision using administrator SSH. Do not
replace only the controller: it requires its accompanying module. Keep the old
root-owned helper for recovery. Production uses its own production_media hooks and
protected resources, never staging hooks. Install the complete reviewed helper set;
staging preparation alone does not establish production photo readiness (see below).

## Staging host prerequisites

For a photo-enabled backend, use that selected revision's
`scripts/provision-photo-decoder.sh` on the authorized Ubuntu 24.04 ARM64 host.
Dependencies installed on an Actions runner do not provision the VPS. Suppress
unrelated service restarts (`NEEDRESTART_MODE=l`) and inspect the package plan;
never reboot or restart production as part of staging preparation.

Provision `/var/lib/international-drift-hub-staging/media` and its dedicated parent
as `idh-staging:idh-staging` mode 0700. Add the absolute `MEDIA_DIR` to the protected
staging backend environment file. The persistent directory must be outside
release/static roots and distinct from all production storage. Add only:

```ini
# /etc/systemd/system/international-drift-hub-staging.service.d/media.conf
[Service]
ReadWritePaths=/var/lib/international-drift-hub-staging/media
```

Keep `ProtectSystem=strict`, unprivileged runtime identity, no capabilities and
other existing sandbox restrictions. Run `systemctl daemon-reload`; the next
staging activation loads the drop-in. The helper runs the candidate binary's
`--check-media` as this runtime user with its protected EnvironmentFile, strict
sandbox and the same writable path **before backup/migrations**. This checks
writeability and an embedded actual HEIC decode, without requiring GitHub to know
application secrets. A failure aborts before migrations or activation.

In staging nginx's existing API/auth/admin proxy location retain original URI and
loopback `127.0.0.1:18081`, and add:

```nginx
if (-f /run/rc-hub-staging-maintenance) { return 503; }
client_max_body_size 26m;
client_body_temp_path /var/lib/nginx/idh-staging-body 1 2;
proxy_read_timeout 120s;
```

Create that private temporary-body directory as `www-data:www-data` 0700. Never
alias media into nginx. Validate `nginx -t` before reload and verify production
health/configuration after the shared nginx reload.

## Coherent snapshots

Provision a randomly generated, root-only mode-0600 encryption passphrase at
`/etc/international-drift-hub-staging/backup.key`. Never print it or put it in Git,
Actions, artifacts or logs. Recovery requires this key: securely escrow it
separately from snapshots. Do not overwrite an existing key.

The deployment and daily timer take the same staging lock. A snapshot:

1. Creates the nginx maintenance flag, rejects new API/auth/admin traffic, drains
   the documented 45-second request-body deadline and stops only the staging Go
   service, pausing reconciliation/cleanup.
2. Creates a PostgreSQL 17 custom-format dump (all metadata, receipts and deletion
   outbox) and a matching media tar excluding abandoned `.upload-*` paths.
3. Records one snapshot ID, staging identity, active BE/FE SHAs, actual goose
   schema version, installed decoder package versions and DB/media SHA-256 hashes.
4. Combines `database.dump`, `media.tar`, `manifest.json`; encrypts using OpenSSL
   AES-256-CBC with salted PBKDF2/200000 iterations and the protected key; decrypts
   to a private temporary archive and verifies DB/media hashes and dump listing.
5. Publishes `/var/backups/international-drift-hub-staging/{predeploy,daily}-UTC.tar.enc`
   and its ciphertext `.sha256` sidecar, restarts the previously running staging
   service and removes maintenance only after both components verify.

Temporary plaintext is confined to root-only backup scratch space and removed.
Failure leaves staging stopped/maintenance closed, exits nonzero and prevents
migration/activation. Inspect the cause and manually recover staging after a
failed snapshot; do not remove the gate to conceal incomplete backups. There is
no automatic database rollback. Retain seven successful daily sets and all
predeploy sets for 30 days, pruning matched ciphertext/checksum pairs only after a
newer verified snapshot exists. Old DB-only dumps are not coherent photo backups.

Restore only to an isolated database and private media directory first. Verify the
ciphertext checksum; decrypt using the protected key with the same OpenSSL options;
verify manifest DB/media hashes; restore the custom dump with PostgreSQL 17
`pg_restore`, and media from the same snapshot without merging current files.
Fix ownership/0700 and run `--check-media`. Verify photo/full/thumbnail records,
covers, receipts, outbox and authenticated reads before relying on recovery.
Never restore this staging snapshot over production.

## Candidate diagnostics and tests

Candidates keep loopback `127.0.0.1:18181` and the existing 20 attempts/5-second
HTTP timeout. Staging adds only the dedicated writable media mount. On failure,
bounded candidate journal entries since this invocation are printed before cleanup,
with environment values and URL-embedded DB passwords redacted. Cleanup suppresses
an already-collected transient unit's harmless stop error. Activation and health
checks remain mandatory; neither active links nor the release receipt are switched
when candidate health fails. This does not add application/database rollback.

Run from repository root:

```sh
python3 -m unittest discover -s scripts/server -v
```

These tests use isolated temporary paths/mocks and do not contact a real host or
read application secrets. Backend CI runs them in addition to existing Go checks.

## Production host preparation

Staging and production use separate resources/hooks. Production has a separate
`production_media.py`, installed root:root 0644 in the same
root-owned helper directory. Install it alongside the updated controller; do not
replace or change the staging module. `production-backup.sh` is installed root:root
0700 at `/srv/international-drift-hub/ops/backup-postgres.sh`, keeping the existing
production timer schedule. Both snapshot jobs and deployment use their respective
environment lock. The main-history gate remains mandatory for production.

Production prerequisites deliberately use different resources:

| Resource | Production |
| --- | --- |
| Runtime user/group | `international-drift-hub` |
| Private media, owner mode | `/var/lib/international-drift-hub/media`, runtime 0700 |
| Environment | `/etc/international-drift-hub/backend.env`, root 0600 |
| Encryption key | `/etc/international-drift-hub/backup.key`, root 0600 |
| Writable systemd mount | `ReadWritePaths=/var/lib/international-drift-hub/media` |
| Backend unit | `international-drift-hub.service` |
| Candidate loopback | `127.0.0.1:18180` |
| nginx body-temp owner mode | `/var/lib/nginx/idh-production-body`, www-data 0700 |
| Maintenance flag | `/run/rc-hub-production-maintenance` |
| Snapshot directory | `/var/backups/international-drift-hub`, root 0700 |
| Compose file/project | `/srv/international-drift-hub/ops/postgres.compose.yml`, `international-drift-hub` |

Set production MEDIA_DIR only to its dedicated path. Add its ReadWritePaths in
`/etc/systemd/system/international-drift-hub.service.d/media.conf`; keep strict
sandbox/security properties. Configure production API/auth/admin nginx location
with the same 26m/120s settings as staging but its own body-temp and maintenance
paths. Validate nginx before reload. Never point production at staging data, key,
media, runtime user or database.

The encryption format, coherent drain/stop/snapshot/verify/reopen sequence,
retention and fail-closed recovery procedure are identical to staging, with the
production module's fixed constants. Escrow the **production** encryption key
separately; it is never emitted, stored in Git, or sent to Actions. Test decrypt,
checksums, PostgreSQL restore and media recovery using a disposable DB/private
directory, never by restoring over production or staging. A snapshot without photo
records cannot establish populated-gallery recovery; that remains a QA gate.

Preparing the host may briefly restart the **same** active backend for a coherent
snapshot/drop-in application. It does not switch release symlinks, apply migrations,
or deploy a new release. After preparation, the approved Actions deployment can
retry its already-uploaded immutable SHA pair; the helper verifies main ancestry,
media preflight and coherent pre-migration snapshot before candidate health and
activation. Candidate failure now emits redacted journal diagnostics in either
environment; a collected unit's harmless cleanup error is suppressed.
