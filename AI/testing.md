# Backend verification gates

Use the version in [go.mod](../backend/go.mod). Unit tests use Go testing/httptest;
persistence and authenticated HTTP tests use real disposable migrated PostgreSQL
through internal/database/dbtest, not SQL mocks or a live database. Google/OIDC is
faked/local-signed, so normal automated tests do not require a Google account.

## Agent-side verification

These shared tiers select development feedback, **never exemptions from mandatory
PR CI**. Choose by the highest affected risk and record the tier/reason in the PR.
Add/update tests for every behavior change; broaden local checks when risk warrants.
Frontend maps these tiers to its own testing commands.

| Tier | Scope | Agent iteration |
| --- | --- | --- |
| Light | Small localized UI/refactor/docs changes | Affected tests and lightweight format/static checks; docs-only changes use the checks below |
| Standard | Feature changes | Changed-package tests, affected integration/contract tests and appropriate static checks |
| Critical | Security/auth/visibility/privacy, data loss/migrations/JSONB compatibility, photo/media, infrastructure/deployment | Targeted integration/security checks locally when feasible, plus all relevant mandatory CI; report unavailable prerequisites |

For Go changes, run `gofmt` on changed Go files and, from `backend/`,
`go test ./internal/<changed-package>` (use `-run <affected-test-pattern>` for a
focused iteration where appropriate). Include affected callers and contract tests.
Persistence/authorization changes require relevant integration tests with disposable
migrated PostgreSQL through dbtest; `go test -short` is never DB integration proof.
Provision codecs only when running tests/builds that need them. Do not automatically
repeat the full test/race/build sequence, codec setup or ARM64 checks each edit.

## Commands

### Documentation and instruction checks

For docs/instruction-only changes, run from repository root:

```sh
python3 scripts/verify-docs.py
python3 -m unittest discover -s scripts -p 'test_docs.py' -v
python3 scripts/verify-engineering-skills.py
python3 -m unittest discover -s scripts -p 'test_engineering_skills.py' -v
git diff --check
```

## Mandatory full GitHub Actions checks

Every PR still runs `.github/workflows/ci.yml` with unchanged triggers and job
identities: `docs-skills`, `test`, `race`, `photo-arm64`. The `test` job also runs
host/helper safety tests, module integrity, formatting and build. Full test/race
checks use disposable PostgreSQL and pinned native codecs. The complete sequence
below belongs to CI (or a justified broader local verification), not every agent
edit cycle. Run from `backend/` with Docker and native codecs available:

```sh
go mod download
go mod verify
test -z "$(gofmt -l .)"
sudo ../scripts/provision-photo-decoder.sh
go test ./...
go test -race ./...
go build ./...
git diff --exit-code -- go.mod go.sum
```

Each isolated CI job must provision the pinned Ubuntu 24.04 decoder before Go
photo tests; installation in another job/host is not enough. Missing codecs fail,
never silently skip. Required jobs remain test, race and photo-arm64; the native
ARM64 job runs internal/photos and CGO_ENABLED=0 Linux/ARM64 build. -short skips
PostgreSQL integration, so cannot establish full correctness. If prerequisites are
unavailable locally, report that and use current-head CI evidence, not a false pass.

Host/helper changes additionally run from repository root:
`python3 -m unittest discover -s scripts/server -v`. This covers both environments'
path rejection, isolation, strict-sandbox preflight, coherent backup before migration,
fail-closed snapshots, journal redaction, candidate-health failure and production
main-SHA gate; tests use temporary paths/mocks, never a live host.

## Mandatory behavior coverage

| Area | Required cases |
| --- | --- |
| Visibility | Product matrix; pending/unfriend revocation; identical missing/private 404; SQL filtering and authorization before protected decode |
| Friendship | Self and duplicate/opposite unordered pairs; only addressee accepts; participant-only mutation and concurrency |
| Catalog | Non-admin denial, active-only selection, inactive historical display, Custom/null, uniqueness |
| Setup JSON | Legacy/schema versions; zero vs unknown; strict JSON, finite/positive constraints, null/empty pruning and full replacement |
| Arm spacing | Both axles/arms/positions; decimals, partial/all-zero, removal/legacy/title-only preservation; real JSONB persistence and invalid writes |
| Photo processing | Actual HEIF/JPEG/PNG/WebP, EXIF orientation, phone-resolution HEIF/JPEG, HEIF export/MIME-name mismatch; corrupt/unsupported, 25 MiB and 50 MP boundaries, no upscaling, stripped metadata, safe keys, resource/cancel failures |
| Photo persistence | Receipts/idempotent retry/conflict, concurrent count/cover, lists without N+1, delete/cascade/outbox/restart and coherent populated DB+media restore |
| Crops | Integer/bounds/overflow, portrait/square/landscape/tiny, exact normalized frame, no full-byte changes, revision retries/conflicts, locks/races, failure cleanup, orphan/version sweeping, all cover projections |
| Transport/security | OpenAPI validity and representative DTO/request/status coverage; 401/origin/admin CSRF/owner checks, no-store/nosniff/current-authorized media, safe errors/no leaked keys |

Keep behavior tests near their owner instead of copying a changing test-name inventory
here. Fixture licenses/provenance remain beside fixtures. Include failure and nearby
invariant cases for every behavior change; record failing-before/passing-after evidence
only if actually reproduced. Packaging/link checks do not prove Cloud behavior.

## Separate live QA

After separately authorized staging deployment, record exact BE/FE SHAs, health and
results in the owning PR/Issue. Exercise login, catalog, setup CRUD/zero/decimals,
public discovery, direct profiles, friendship accept/remove and public/friends/private
reads with two accounts. Changed photo/crop paths require original iPhone HEIF and
rotated JPEG, one/ten/eleven photos, interrupted retry, crop/cover/delete/reload,
unsaved technical-data preservation, known media URLs/unfriend and real resampling.

At 320/375/390/768/1280 and short landscape check overlap, focus, labels, keyboard and
touch. Mocked browser/native fixture tests do not establish real device/OAuth/live QA.
For host work also run the exact candidate --check-media as the runtime user under
the strict sandbox and verify an isolated coherent DB+media restore, public health
and unaffected peer environment. Recovery/deployment remains [explicit](deployment.md).

For fresh context/skills use [context smoke](context-smoke.md) and
[engineering skill smoke](engineering-skills.md); keep transcripts and limitations in PRs.
