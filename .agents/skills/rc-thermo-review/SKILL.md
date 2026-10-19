---
name: "rc-thermo-review"
description: "Perform a strict read-only architecture quality audit for complexity, abstraction, ownership, and maintainability regressions. Use for deep or Thermo-Nuclear review."
license: "MIT"
---

# Thermo-Nuclear architecture quality review

Read applicable AGENTS.md, AI/architecture.md, AI/decisions.md and relevant product/testing docs.
Review the stated scope plus connected ownership boundaries, not the whole repository by default.
Analyze independently of implementation claims. Read-only means no code changes, auto-fixes,
scaffolds, commits, remote comments/approvals, merges, deployments or broad-refactoring authority.

Look for simplifications that delete concepts, state duplication, special cases or layers
rather than moving complexity into more files. Examine cohesive ownership, explicit type
contracts, canonical helper reuse, information leakage, feature logic in shared paths,
branch growth, unnecessary wrappers/casts and partially applied related updates. Prefer the
project's existing architecture to speculative abstractions. Never remove security,
validation, accessibility, required tests or deliberate compatibility to simplify code.

Treat file growth near 1,000 lines as a prompt to inspect cohesion, not an automatic blocker;
exclude generated/vendor output and justify decomposition by responsibility and change risk.
Unnecessary serialization may be a smell, but do not propose parallelism without checking
ordering, shared state and failure semantics. Strict means evidence-backed, not more findings.

Backend focus: preserve the single Go service, typed setup schema and PostgreSQL.
Keep wiring in backend/cmd, domain rules in backend/internal and SQL at repository boundaries.
Preserve backend-owned OAuth/HttpOnly sessions, visibility/ownership checks, zero versus omitted
values, OpenAPI transport fidelity and private photo metadata/media consistency. Use real
PostgreSQL and supported native decoders where AI/testing.md requires them; short tests alone
do not establish integration, race, ARM64, backup/restore or live QA correctness.

Prioritize structural regressions and demonstrated simplifications. For each finding provide
file/line evidence, the complexity or boundary failure, practical cost, a bounded remedy,
behavior-preservation conditions and verification needs. Separate required findings from
optional redesign ideas outside scope. A plausible rewrite alone is not a blocker.
If no actionable regression is demonstrated, say so. This is an architecture quality report,
not a full security certification or merge authorization. Use a fresh session for independent review.

Adapted sources, revisions and licenses: [engineering skills](../../../AI/engineering-skills.md) (cursor-plugins).
