# Backend verification gates

Use the version in [go.mod](../backend/go.mod). Unit tests use Go testing/httptest;
persistence and authenticated HTTP tests use real disposable migrated PostgreSQL
through internal/database/dbtest, not SQL mocks or a live database. Google/OIDC is
faked/local-signed, so normal automated tests do not require a Google account.

## Commands

For docs/instruction-only changes, run from repository root:

```sh
python3 scripts/verify-docs.py
python3 -m unittest discover -s scripts -p 'test_docs.py' -v
python3 scripts/verify-engineering-skills.py
python3 -m unittest discover -s scripts -p 'test_engineering_skills.py' -v
git diff --check
```

For application changes run Backend CI's sequence from backend/ with Docker and
native codecs available (full CI still runs for every PR):

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
