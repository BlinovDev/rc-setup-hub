---
name: "rc-implement"
description: "Implement an authorized feature or bug fix with reuse, simplicity, and the smallest complete change. Use for implementation, not independent review."
license: "MIT"
---

# Minimal complete implementation

Read the task, applicable AGENTS.md files and the project's required AI documents first.
Implement only the approved acceptance criteria; preserve accepted decisions and all verification gates.

Before editing, trace the affected entry point, canonical owner, callers, tests and fixtures.
Choose in order: existing repository capability, standard platform/library capability,
already-installed dependency, then the smallest direct implementation. Explain a needed
new abstraction or dependency; do not add speculative options or a parallel implementation.
Fix a shared cause at its owner rather than patching every consumer. Complete the required
callers, error paths, fixtures and documentation; a short but incomplete diff is not a solution.

Keep validation, error handling, privacy, accessibility and data integrity intact. Do not
trade required tests for a smaller diff. Use the regression workflow when behavior changes.
If a proposed simplification would alter business rules or expand scope, report it separately.

Backend focus: preserve the single Go service, typed setup schema and PostgreSQL.
Keep wiring in backend/cmd, domain rules in backend/internal and SQL at repository boundaries.
Preserve backend-owned OAuth/HttpOnly sessions, visibility/ownership checks, zero versus omitted
values, OpenAPI transport fidelity and private photo metadata/media consistency. Use real
PostgreSQL and supported native decoders where AI/testing.md requires them; short tests alone
do not establish integration, race, ARM64, backup/restore or live QA correctness.

Report the concrete behavior changed, relevant checks actually run, and unresolved risks.
An implementation self-check is not an independent review. Do not silently switch into
review/approval mode. This skill grants no merge, deployment or broad-refactoring authority.

Adapted sources, revisions and licenses: [engineering skills](../../../AI/engineering-skills.md) (ponytail).
