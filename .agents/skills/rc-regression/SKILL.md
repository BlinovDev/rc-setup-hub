---
name: "rc-regression"
description: "Reproduce a bug or add behavior-focused regression tests during an authorized fix. Use for TDD and failing-before/passing-after evidence."
license: "MIT"
---

# Regression testing

Read applicable AGENTS.md and AI/testing.md; preserve every required verification gate.
Identify intended versus observed behavior and the nearest existing test boundary.
For a bug fix, write a focused test that fails for the actual bug before editing the fix,
then make the smallest complete correction and rerun it plus required surrounding checks.
If the test passes initially or fails from missing tooling, that is not failing-before evidence.

Prefer observable results and real boundaries over mocks of implementation details. Exercise
negative paths and nearby invariants affected by the change. Do not rewrite assertions to
accept a defect, silently skip missing prerequisites, or replace required integration/browser
coverage with a convenient unit test. If reproduction is unavailable, report the exact missing
prerequisite and strongest actual evidence; do not claim red/green or completion.

Backend focus: preserve the single Go service, typed setup schema and PostgreSQL.
Keep wiring in backend/cmd, domain rules in backend/internal and SQL at repository boundaries.
Preserve backend-owned OAuth/HttpOnly sessions, visibility/ownership checks, zero versus omitted
values, OpenAPI transport fidelity and private photo metadata/media consistency. Use real
PostgreSQL and supported native decoders where AI/testing.md requires them; short tests alone
do not establish integration, race, ARM64, backup/restore or live QA correctness.

Use disposable local fixtures/data only. This workflow permits test/fix edits only within
an already-authorized implementation task; a request for a test plan alone remains analysis.
Report test names, failing-before cause, passing-after results, broader checks and limitations.
No merge, deployment or broad refactor is authorized by this skill.

Adapted sources, revisions and licenses: [engineering skills](../../../AI/engineering-skills.md) (pstack).
