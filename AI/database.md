# Database design

## Sources and ownership

[SQL migrations](../backend/migrations) own tables, constraints, indexes and foreign
keys. [Typed setup models](../backend/internal/setups/model.go) and
[OpenAPI schemas](../backend/api/openapi.yaml) own current schema-v1 fields. Read
those files for an affected field instead of maintaining a second schema or JSON
example here. [Product](product.md) owns business rules and the visibility matrix;
[security](architecture.md#authentication-and-security) owns authorization.

PostgreSQL uses UUID identities and TIMESTAMPTZ/UTC. Current storage boundaries:

| Storage | Canonical definition | Responsibility |
| --- | --- | --- |
| Users | [00001](../backend/migrations/00001_users.sql) | Stable Google subject; case-insensitive unique email/nickname; no admin role column |
| Friendships | [00002](../backend/migrations/00002_friendships.sql) | Unique unordered user pair, no self-pair; user deletion cascades |
| Chassis catalogs | [00003](../backend/migrations/00003_chassis_brands.sql), [00004](../backend/migrations/00004_chassis_models.sql) | Case-insensitive names; model brand FK restricts deletion; disable historical references |
| Setups/search | [00005](../backend/migrations/00005_setups.sql) | Relational owner/chassis/visibility plus typed JSONB, not a generic JSON bag |
| Photo metadata/receipts/outbox | [00006](../backend/migrations/00006_setup_photos.sql), [00007](../backend/migrations/00007_photo_crops.sql) | Private file references and durable cleanup, detailed below |

A NULL chassis means custom/unlisted/unspecified, not a synthetic catalog row.
Deleting a user cascades setups; physical model deletion sets setup chassis NULL.
Visible comparison selection also uses the cross-visibility `(created_at DESC,
id DESC)` keyset index from [00008](../backend/migrations/00008_visible_setup_order.sql),
existing owner/visibility indexes and the unique unordered friendship-pair index.
Both comparison sides authorize in one statement snapshot before full projections;
no persistent comparison table or write transaction is introduced.
Do not add a general JSONB GIN index without an actual JSON-field search requirement.

## Schema-v1 evolution and atomicity

Optional numbers remain pointer-backed: zero is a value, not absence. Supplied data
PATCH replaces the entire technical document; legacy/unknown/explicit-zero and
historical reference behavior must survive round trips. Empty new optional blocks
are normalized before writes without reinterpreting unrelated legacy parents.
Optional arm spacing, differential, shock hardware, knuckle, arm mounts and running
gear live in the existing JSONB; no new columns/backfill were required for those
extensions. Exact fields and validation belong to models/OpenAPI and their tests.

Incompatible JSON evolution requires a new version, explicit old/new read and
migration strategy, and regression tests before changing write defaults. Do not
silently reinterpret unsupported stored versions.

Repositories own SQL. Use transactions for dependent writes/checks; ordinary
single-row CRUD needs no extra transaction solely for uniformity. Photo mutations
and their row-lock/publication/cleanup ordering below are deliberate exceptions.
Use real disposable migrated PostgreSQL for JSONB, constraints, indexes and race
coverage; commands/prerequisites live in [testing](testing.md).

## Setup photos (migration 00006)

setup_photos stores UUID id/setup FK ON DELETE CASCADE, random generated storage
key, full/thumbnail dimensions, is_cover and created_at. No bytes in PostgreSQL;
no photos in technical JSONB. Cover has a partial unique index and deferred
constraint trigger requiring exactly one for nonempty galleries and <=10 photos.
Every owner photo write first locks the parent setup FOR UPDATE; count checking
and cover/fallback changes are serialized even for concurrent ninth/tenth uploads.
Stable order is created_at,id. Cover joins use the unique partial cover index.

setup_photo_uploads holds setup-scoped UUID request_id, source SHA-256 and photo
FK ON DELETE SET NULL. Same bytes/key reuse the existing record; changed bytes or
a tombstoned deleted photo conflicts. Receipts cascade only with setup deletion.
setup_photo_deletions is a durable outbox populated by an AFTER DELETE trigger,
including setup cascades. Successful cleanup removes the outbox row; failure or
restart retains it for retry. Full/thumbnail publication is an atomic directory
rename before the metadata transaction. Commit-outcome uncertainty retains files;
retry reads the receipt. Unreferenced publication/temp directories older than one
hour are reconciled; never sweep unless the DB inventory query succeeded.

Normal startup reconciles before accepting requests; a one-minute worker retries
the complete deletion queue in keyset pages of 100 before inventory verification
and orphan sweeping. Individual file failures retain their rows without blocking
later pages; any cleanup failure still fails startup closed. Delete and changed-crop
requests attempt only one page, without global inventory checks or sweeping;
no-op crops do no cleanup. Storage failures never silently create usable-looking
photo metadata. Private runtime-owned storage is outside release/static roots;
restores must include consistent database AND media, not DB-only backups.
Migration down drops photo metadata/receipts/outbox and is destructive; archive
DB/media before any rollback. It does not delete filesystem files automatically.

## Thumbnail crops (migration 00007)

Adds nullable JSONB crop, INTEGER thumbnail_revision default 0, nullable safe
thumbnail_key to setup_photos. Crop stores integer x/y/size with DB bounds checks
against full dimensions. Old rows and media are backwards readable, with no backfill.
New uploads persist center crop/revision 1; full/storage_key never changes.
Adjusted thumbnails live in separate immutable random-key directories with only
thumbnail.jpg; COALESCE(thumbnail_key,storage_key) resolves legacy/default media.
All joined cover projections include crop/revision; internal keys stay private.

Parent setup FOR UPDATE serializes crop/cover/delete and upload metadata. Crop
holds it through bounded generation; file is fsynced/atomically published before
metadata commit. Confirmed precommit failures clean new files; uncertain commits
retain them, with identical retry/version checks and one-hour orphan reconciliation.
UPDATE thumbnail_key queues superseded version directories; DELETE/cascade queues
full and current thumbnail keys. Durable cleanup retries failures. Reconciliation
checks referenced full/current thumbnail files and retains both directory types;
missing referenced media fails startup. Legacy thumbnail.jpg is kept with full.jpg
for compatibility/rollback. Down drops crop metadata, reverting to that baseline;
archive coherent DB/media before rollback. Backup/restore must retain current version
directories, not just full-image keys; no media relocation/new storage is introduced.
