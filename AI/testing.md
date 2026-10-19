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

## Host deployment regression checks

Run `python3 -m unittest discover -s scripts/server -v` from repository root.
Backend CI runs this alongside Go checks. It verifies staging media path rejection,
runtime/sandbox preflight, preflight+backup ordering before migrations, fail-closed
snapshot behavior, candidate journal redaction and unchanged production hook
selection. Tests use temporary paths and mocks; they never access live environments.
Host QA must additionally run the exact ARM64 candidate's `--check-media` under the
staging sandbox, verify a coherent snapshot/isolated restore, deploy the explicitly
approved SHA pair and verify public staging/backend health and unchanged production.

### Photo decoder provisioning in every CI job

Each GitHub Actions job has an isolated runner. The regular test, race and native
ARM64 photo jobs must each run `sudo ../scripts/provision-photo-decoder.sh` from
`backend/` before their Go tests. Installation in another job or on Hetzner does
not provide `convert` to this runner. Keep Ubuntu 24.04 runner labels aligned with
the supported pinned provisioning script; missing codecs must fail tests rather
than skip photo coverage.

## Square crop stage-2 checks (Issue #10)

TestCropBounds covers integer/bounds/zero/negative/overflow and odd center offsets.
TestSquareCropNormalizedSourceAndPublication covers portrait 3:4/9:16, square,
landscape 4:3/16:9, tiny images, exact colored frame selection, no-upscaling,
full SHA256 preservation, failed/cancelled generation and orphan/version sweeping.
Existing real JPEG EXIF orientation and HEIC/PNG/WebP processing checks now require
square default thumbnails based on normalized full dimensions.
TestCropPersistenceRetriesRacesAndCleanup uses migrated PostgreSQL for defaults,
legacy proportional media, reset/round-trip/all list projections, idempotent retries,
stale edits, two-frame concurrency, generation/SQL publication failures, unchanged
full bytes, cover/technical preservation and crop/deletion serialization/cleanup.
Photo HTTP tests exercise strict/missing/null/fraction/overflow/string/unknown
request rejection, 401/403/non-owner checks, retries/409 and current thumbnail
privacy for private/friends/public/unfriend; backup restore includes a committed
crop/version directory. OpenAPI validates the endpoint, typed request/response and
all cover transports. Required full/race/native ARM64 CI and host safety tests remain.

Local short tests alone do not verify PostgreSQL or pinned decoder provisioning.
Use the exact CI result on the PR as evidence for those gates if local Docker/codecs
are unavailable. No missing decoder test may be silently skipped. FE roadmap and
320/375/390/768/1280 browser matrix, physical touch/iPhone HEIC and live two-account
staging QA are pending linked Issue #13; no deployment is authorized by Issue #10.

Production host regression coverage reuses the storage/snapshot failure and
redaction cases against the separate production module and checks resource
isolation. Controller cases require preflight and coherent backup before migrations
in both environments, retain the production main-SHA gate, and reject candidate
activation on failed health. Native host preparation additionally runs the exact
production candidate's `--check-media` as the runtime user under the strict
sandbox and restores the encrypted matched snapshot into disposable resources.

## Engineering skill foundation checks

For repository-local instruction/provenance changes, run
`python3 scripts/verify-engineering-skills.py` and
`python3 -m unittest discover -s scripts -p 'test_engineering_skills.py' -v`
from repository root. These validate packaging, not Codex runtime behavior.
Fresh Cloud discovery/invocation smoke prompts and explicitly unverified capabilities
are documented in `AI/engineering-skills.md`. Existing application verification
requirements above remain unchanged.
