---
name: "rc-blast-radius"
description: "Analyze what a proposed change or diff can break beyond its immediate callers. Read-only impact analysis with explicit evidence and unproven assumptions."
license: "MIT"
---

# Blast-radius analysis

Read applicable AGENTS.md and relevant AI architecture, API, decisions and testing docs.
Establish the exact diff/base or proposed change before reasoning about impact. Trace callers
and consumers, then cross boundaries that symbol search misses: persisted representations,
wire formats, caches, lifecycle ordering, cleanup, generated contracts and paired releases.
Do not assume another repository is checked out or authorized for inspection.

Find the one or two facts on which safety depends. For each, distinguish an assertion,
source evidence, a traced counterexample, executable evidence and live QA. Run an existing
focused check only when it uses disposable local resources and is allowed; otherwise name
the proof still needed. Grep and a convincing explanation alone are not runtime proof.
Do not execute live services or mutate production/staging to obtain stronger evidence.

Backend focus: preserve the single Go service, typed setup schema and PostgreSQL.
Keep wiring in backend/cmd, domain rules in backend/internal and SQL at repository boundaries.
Preserve backend-owned OAuth/HttpOnly sessions, visibility/ownership checks, zero versus omitted
values, OpenAPI transport fidelity and private photo metadata/media consistency. Use real
PostgreSQL and supported native decoders where AI/testing.md requires them; short tests alone
do not establish integration, race, ARM64, backup/restore or live QA correctness.

Return changed behavior, confirmed downstream risks with trigger/impact/file evidence,
checked-and-cleared paths and unproven safety assumptions with the smallest useful checks.
Read-only: do not edit source, tests, fixtures or commit an experimental script. Do not
invent callers or numerical likelihoods. No automatic fix, merge, deployment or broad refactor.

Adapted sources, revisions and licenses: [engineering skills](../../../AI/engineering-skills.md) (pstack).
