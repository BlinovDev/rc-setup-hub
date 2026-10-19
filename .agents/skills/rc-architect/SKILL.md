---
name: "rc-architect"
description: "Analyze architecture, ownership, interfaces, and design alternatives before a non-trivial change. Read-only analysis; does not start implementation."
license: "MIT"
---

# Architecture analysis

Read applicable AGENTS.md, AI/architecture.md, AI/decisions.md and the relevant product/API docs.
Trace the existing flow from caller through the canonical owner to external boundaries.
Describe why the current boundary exists using code and accepted decisions, not guesses.

Sketch the smallest viable design in the report: caller usage, types/signatures, ownership,
state transitions and failure handling. For a genuine structural tradeoff compare distinct
viable shapes, including reuse of the current design; do not manufacture alternatives for
trivial work. Judge information hiding, cohesive ownership, coupling and testability.
Look for split state ownership, pass-through layers, hand-synchronized representations,
and callers forced to coordinate internal stages. Do not replace project-approved contracts
with an upstream preference for a different type model or architecture.

Backend focus: preserve the single Go service, typed setup schema and PostgreSQL.
Keep wiring in backend/cmd, domain rules in backend/internal and SQL at repository boundaries.
Preserve backend-owned OAuth/HttpOnly sessions, visibility/ownership checks, zero versus omitted
values, OpenAPI transport fidelity and private photo metadata/media consistency. Use real
PostgreSQL and supported native decoders where AI/testing.md requires them; short tests alone
do not establish integration, race, ARM64, backup/restore or live QA correctness.

Return evidence with file locations, constraints, tradeoffs, a recommended bounded design,
verification needs and unresolved decisions. Distinguish proposed design from existing facts.
Read-only: no scaffolds, edits, generated artifacts or implementation commits. No background
model orchestration or assumed companion tools. Analysis never authorizes implementation,
merging, deployment, or broad refactoring; those require an authorized task.

Adapted sources, revisions and licenses: [engineering skills](../../../AI/engineering-skills.md) (pstack).
