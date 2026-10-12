# Testing strategy

Testing is used as a progress gate for vibe-coded development.

Every roadmap phase has a small observable outcome and tests that prove it.

## Test layers

### 1. Unit tests

Use Go's standard `testing` package.

Best targets:
- setup JSON validation;
- visibility decisions;
- friendship state rules;
- admin email allowlist parsing/checking;
- request validation;
- pagination/cursor helpers if introduced.

Prefer table-driven tests.

### 2. Repository integration tests

Repository SQL must be tested against real PostgreSQL.

Reason: this project depends on PostgreSQL-specific behavior:
- JSONB;
- UUID;
- expression indexes;
- constraints;
- foreign keys;
- `TIMESTAMPTZ`.

Do not rely only on SQL mocks.

Recommended implementation:
- run PostgreSQL in Docker;
- use `testcontainers-go` or a dedicated isolated test database;
- apply real migrations before repository tests;
- clean data between tests.

The exact choice can be made in milestone 1/2. Favor the option that makes `go test ./...` repeatable for a developer with Docker running.

### 3. HTTP tests

Use `httptest`.

Test:
- status codes;
- authentication requirements;
- authorization;
- malformed request handling;
- response shape for critical endpoints.

Do not require a real Google login for most handler tests. Wrap identity/session resolution behind a narrow boundary that tests can substitute.

### 4. End-to-end smoke tests

Add only after core API is implemented.

Minimum automated/manual smoke path:
1. login/test session;
2. list chassis catalog;
3. create setup;
4. read own setup;
5. make it public;
6. find it through public search;
7. create second user;
8. send/accept friendship;
9. switch setup to friends;
10. friend can read it;
11. unrelated user cannot;
12. switch to private;
13. only owner can read it.

## Mandatory regression tests

### Visibility matrix

For each visibility:

| Caller | public | friends | private |
|---|---:|---:|---:|
| owner | allow | allow | allow |
| accepted friend | allow | allow | deny |
| unrelated authenticated user | allow | deny | deny |

This logic must have table-driven tests.

### Friendship uniqueness

Test:
- A -> B pending succeeds;
- another A -> B fails;
- B -> A while A -> B exists fails;
- self-request fails;
- only B can accept A -> B.

### Chassis catalog

Test:
- ordinary user cannot mutate catalog;
- admin can create brand/model;
- inactive model does not appear in regular selectable catalog;
- an existing setup referencing an inactive model still renders correctly;
- setup can use `chassis_model_id = NULL`.

### Setup JSON

Test:
- `0` degrees survives encode/decode as a real value;
- omitted degree remains omitted;
- valid schema v1 persists and loads;
- unsupported schema version is handled explicitly.

## Commands

The final repository should converge on simple commands such as:

```bash
go test ./...
go test -race ./...
```

Later add a `Makefile` or task runner only if it meaningfully simplifies repeatable commands.

## Arm spacing regression coverage

`TestArmSpacingNormalization`, `TestArmSpacingValidation` and
`TestArmSpacingStrictDecode` cover legacy shapes, explicit zero, decimals, partial
unknown values, null/empty blocks, parent pruning, all axle/arm/value positions,
non-finite/negative values and strict unknown-field/numeric-string rejection.
`TestArmSpacingHTTPPersistence` uses disposable migrated PostgreSQL through the
existing dbtest helper: create/PATCH/detail/full lists and repository reads,
actual JSONB 0/5.2, title-only preservation, removal/clearing, invalid writes,
and legacy documents. API transport tests validate spacing DTOs and requests
against the embedded OpenAPI; existing visibility/ownership tests remain required.

Run Backend CI commands from backend/: `go mod download`, `go mod verify`,
`test -z "$(gofmt -l .)"`, `go test ./...`, `go test -race ./...`,
`go build ./...`, `git diff --exit-code -- go.mod go.sum`. Go version comes from
go.mod; Docker must be running for real PostgreSQL. `-short` skips integration
tests and is not evidence of complete backend validation.

Paired frontend/live staging QA (including detail labels and 375px/desktop)
requires separate deployment authorization and a compatible explicit BE/FE SHA
pair. Verify 0/5.2, all independent blocks, decimals/partial unknowns, reopen,
unrelated edit, removal/clearing, negative/malformed errors and existing setups.

## Setup photo regression gates (Issue #5)

Photo store tests exercise content detection for JPEG/PNG/WebP/HEIC (real libheif
HEVC fixture with provenance/license), conversion to JPEG, orientation via actual
EXIF, metadata removal, resizing/no upscaling, corrupt/unsupported/empty/encoded
limits, cancellation, safe keys, source/temp cleanup and aged orphan reconciliation.
Startup verifies actual HEIC decode; an advertised HEIC coder without its decoder
plugin is a failure, never a skip/pass. Full native tests run on ARM64 CI as well
as amd64. Packages are pinned by scripts/provision-photo-decoder.sh.

Real PostgreSQL tests cover migration 6, metadata/order/cover projections across
detail/My/profile/search, idempotency conflicts/retries, concurrent uploads at
nine/ten, cover deletion fallback, technical edit independence, setup cascade/file
cleanup, failed filesystem cleanup/outbox retry, auth/owner/origin policy and
public/private/friends thumbnail/full URLs including friendship revocation.
TestPhotoBackupRestore uses actual pg_dump/pg_restore against two disposable
PostgreSQL 17 containers and separate media directories; source deletion must not
affect restored metadata/files. Transport tests validate non-null cover and
gallery DTOs; the embedded OpenAPI/reference validator covers the new routes.

Required checks: formatting, go mod download/verify, go test ./..., go test -race
./..., go build ./..., unchanged modules; native ARM64 photo tests and CGO-disabled
ARM64 build. Docker remains required for integration tests. No production DB or
live server is used. -short omits DB suites and is not full validation.

Remaining paired live QA after separate deployment authorization: legacy setup;
one/ten/eleven images; cover/all-list reloads, remove cover/final, interrupted
upload retry without duplicate setup/photos; iPhone HEIC and rotated actual
photos; two-account visibility/direct full/thumbnail URLs and unfriend; technical
edit independence, delete setup, persistence across releases/restarts and an
isolated production-like backup/restore. 375px/desktop/gallery/focus and the
monochrome bundled placeholder belong to frontend Issue #11. Native fixtures are
not physical iPhone/device or live-server proof.
