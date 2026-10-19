---
name: "rc-review"
description: "Independently review a diff for concrete correctness, security, regression, and maintainability issues. Read-only review; does not fix or approve merging."
license: "MIT"
---

# Independent engineering review

Read applicable AGENTS.md and relevant AI decisions, architecture and testing requirements.
Resolve the exact review scope and base/head revisions. Read the diff, surrounding flow,
callers and tests independently of the implementer's explanation. Existing design decisions
are constraints, not permission to redesign. An implementer's self-check must be labeled as
such; use a fresh review session when independent evidence is needed.

Challenge input validation, authentication/authorization, data integrity, concurrency,
error handling, lifecycle/cache behavior and test adequacy before aesthetic concerns.
Check upstream-inspired simplifications against the project's actual scale and requirements.
For every finding, verify a reachable trigger and observable impact in the current code.
Separate introduced regressions from pre-existing debt, confirmed findings from questions,
and necessary fixes from optional structural suggestions. Never invent a finding quota.

Backend focus: preserve the single Go service, typed setup schema and PostgreSQL.
Keep wiring in backend/cmd, domain rules in backend/internal and SQL at repository boundaries.
Preserve backend-owned OAuth/HttpOnly sessions, visibility/ownership checks, zero versus omitted
values, OpenAPI transport fidelity and private photo metadata/media consistency. Use real
PostgreSQL and supported native decoders where AI/testing.md requires them; short tests alone
do not establish integration, race, ARM64, backup/restore or live QA correctness.

Return highest-impact findings first with severity, precise file/line, trigger, impact,
evidence and the smallest suitable remedy. Include executed checks and missing evidence.
If none are confirmed, say so with residual risks; that is not proof of correctness or approval.
Read-only: no edits, automatic fixes, commits, PR approval, remote review comments, merges,
deployments or broad refactoring. Do not imply independent models ran; no orchestration is assumed.

Adapted sources, revisions and licenses: [engineering skills](../../../AI/engineering-skills.md) (pstack).
