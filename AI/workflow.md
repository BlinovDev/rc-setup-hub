# Delivery and documentation policy

## Sources of truth

| Question | Owner |
| --- | --- |
| Applicable instructions / context routing | Root and nested AGENTS.md; load only task-relevant docs/skills |
| Implemented behavior, commands and storage | Current code, tests, migrations, package scripts and workflows |
| Transport shapes/statuses | Backend [OpenAPI](../backend/api/openapi.yaml); frontend commits an exact copy and regenerates types |
| Shared enduring product rules | Backend [product](product.md); frontend product/UI docs own client-only interaction rules |
| Enduring decisions and rationale | Repository [decisions](decisions.md), linked to affected rules |
| Approved task scope / acceptance criteria / dependencies | Owning GitHub Issue, with linked child issues for repository-specific work |
| Root cause, implementation narrative, test evidence, SHA pairs and QA outcome | PR/Issue, not evergreen Markdown appendices |
| Release operations and recovery | [Deployment](deployment.md) and [host runbook](../scripts/server/README.md) |

Code and machine-readable contracts describe implementation facts; they do not
silently repeal accepted security/product decisions. If these disagree, identify
the discrepancy and resolve it within approved scope; ask for a material product,
authorization, data or compatibility decision instead of guessing. Historical
[archives](archive/README.md) and chat history are not active requirements.

## Docs by exception

Update an evergreen document only when an enduring product rule, public contract,
architecture decision, operational procedure or developer command changes. Edit
its owning section; link to it elsewhere rather than copying the rule. API changes
still require implementation/tests/OpenAPI together and frontend regeneration.
Bug fixes, refactors and ordinary UI polish that restore existing rules may ship
with **zero evergreen-doc changes**; explain unchanged invariants in the PR.

Do not append phase reports, file inventories, test counts, incident narratives,
commit hashes or session-specific pending status to product/API/testing docs.
Put those in Issues/PRs. Preserve fixture provenance/licenses beside fixtures.
No arbitrary line cap: keep unique actionable constraints and remove repetition.

## Scope and implementation

Use approved acceptance criteria before implementation. A focused issue records
the user problem, scope/exclusions, risks/dependencies and tests/QA. Existing explicit
implementation authorization is sufficient; ask only about material ambiguity or
scope expansion. For phased tasks follow the approved issue's gates, not the old POC
roadmap. Work on a feature branch and create a PR against main; no opportunistic work.

Cross-repo features have one specification owner (normally a backend umbrella Issue
for shared behavior) and linked BE/FE child issues with local acceptance deltas.
Do not duplicate the whole specification in both repositories. Backend changes the
contract first; FE consumes the exact feature-PR contract without requiring a merge,
commits its copy/generated types, and links the backend PR. Both PRs name compatible
exact SHAs and any merge/release order. One-sided changes select a compatible peer SHA.

Prefer one fresh feature/task context and one capable agent for BE → FE. Verify
actual read/edit/test/commit/PR capabilities in both repositories first; if missing,
stop and report the limitation. A GitHub connector alone does not launch Codex Cloud.
Create the backend PR first, then immediately continue frontend work in the same
context without waiting for backend merge. For contract changes, fetch OpenAPI at
the full immutable backend PR HEAD SHA; verify the copied bytes, regenerate types
and record that contract SHA and the compatible BE/FE head pair on both PRs. If a
backend revision changes the contract, refresh and reverify the frontend copy.
For contract-unchanged tasks, record the existing contract source SHA/copy equality;
do not churn generated files. Keep separate PRs and independent CI/release gates.

Run [relevant checks](testing.md) and required CI at the current PR head. Missing,
pending, cancelled or failed required checks are not green. Put evidence and
limitations in the PR using the [template](../.github/pull_request_template.md).
A self-check is not independent review. Do not claim a Cloud task/automation exists
without a real returned identity and execution evidence.

## Context, CI and output budget

Read AGENTS.md and relevant owners once; retain a short path/decision/check ledger.
Read additional docs, skills, generated code or history only to resolve a concrete
question. Avoid broad architecture reviews, whole-log reads and unchanged-doc rereads.

Create PRs after relevant local checks; do not wait for full CI to open them. Check
current-head CI once after PR creation (or consume one actual event-driven result).
Do not poll or wait in a loop. Pending remains pending. On failure inspect only the
failed jobs/steps and concise error excerpts; keep full logs/artifacts out of context
unless a specific diagnostic needs them. Allow at most one automatic focused repair
or transient-job rerun and one current-head recheck, then record any remaining
blocker in the PR. Never rerun successful jobs unnecessarily or weaken tests/gates.

Put tier/reason, actual commands/results, missing prerequisites, CI state at the
observed head, contract SHA/pair and QA limitations in concise PR evidence. Final
chat contains only BE PR URL, FE PR URL and unresolved blockers (as applicable).
No long phase reports, repeated file inventories or unverified success claims.

Prompt migration: replace “run all suites locally and wait until CI is green” with
“select the testing tier, run affected checks, open linked PRs, inspect CI once”.
Existing security, compatibility and release gates remain mandatory.

For the next 2–3 features, record tier/scope, tool and test invocation counts,
full versus targeted runs, CI repair cycles and elapsed time in each owning PR.
Compare similar scopes with an observed prior workflow baseline if available;
record token usage only when the harness exposes it. Otherwise mark it unavailable.
Use these observations to adjust the policy; do not invent savings or baselines.

## Staging, merge and production gates

Implementation complete, QA passed, merged and deployed are separate states.
Green CI does not authorize merge or deployment. Staging is shared and explicit,
not automatically replaced by every PR. Only the backend's manually dispatched
[coordinator](../.github/workflows/deploy.yml) deploys an immutable BE/FE SHA pair.
Record selected pair, health and QA evidence on linked PRs; preserve isolated
runtime users/configuration/databases/media and backup/preflight/migration gates.

When separately authorized, staging QA exercises acceptance criteria and affected
security/regression paths; failed QA preserves scope and requires fixes/rechecks.
Only approved changes merged to both relevant main histories may reach production.
Production is separately explicit, uses production configuration, migrations and
health/smoke gates; no feature-branch production release, automatic rollback or
in-flight deployment cancellation. Recovery is administrator-owned. Full operational
requirements remain in [deployment](deployment.md), not this process summary.

Project ideas follow the [Inbox connection and approval gate](project-intake.md).
Repository Issue auto-add and authenticated ChatGPT Project reads are separate capabilities.
