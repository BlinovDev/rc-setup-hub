# RC Setup Hub — task router

Read the task/Issue and [workflow](AI/workflow.md), then only the rows needed below.
Inspect current code, tests and applicable nested AGENTS.md before editing. Do not
load every AI document or historical roadmap. No prior chat context is required.

| Task | Context to load |
| --- | --- |
| Product rule / permission / feature | [Product](AI/product.md), relevant [decisions](AI/decisions.md) |
| Go implementation / architecture | [backend instructions](backend/AGENTS.md), [architecture](AI/architecture.md), [development](AI/development.md) |
| HTTP / auth / contract | [API](AI/api.md), [security boundary](AI/architecture.md#authentication-and-security), affected OpenAPI schemas/tests |
| SQL / migration / media persistence | [Database](AI/database.md), affected migrations/repositories |
| Checks / bug regression | [Testing](AI/testing.md), nearest existing tests |
| Release / host / recovery | [Deployment](AI/deployment.md), [host runbook](scripts/server/README.md) |
| Documentation / instructions | [Workflow](AI/workflow.md), [context smoke](AI/context-smoke.md); edit only affected sources |

## Non-negotiable boundaries

- Implement only approved acceptance criteria; keep changes small and testable.
- Preserve the single Go service, PostgreSQL/pgx, goose and typed schema-v1 JSONB;
  explicit zero is not omitted. No ORM or speculative abstractions.
- Preserve Google-only backend-owned HttpOnly sessions, exact credentialed CORS,
  mutation-origin/admin CSRF checks, owner writes and visibility-filtered reads.
  Never expose secrets, tokens, private data or raw database/decoder errors.
- Preserve image formats/limits, private storage and coherent DB+media safety gates.
- Backend owns [OpenAPI](backend/api/openapi.yaml). Cross-repo changes update it
  first, then the frontend's committed copy/generated types; link both PRs.
- Add/update tests for behavior changes. Report actual checks and missing tooling;
  mocked/short tests are not full PostgreSQL, native ARM64 or physical/live QA.
- Docs by exception: update one canonical source for an enduring change. An ordinary
  fix/refactor with unchanged rules/contracts may have zero evergreen-doc changes.
- Work on a branch; PR against main. Never merge, deploy or touch live systems
  without explicit authorization. Green CI is not release approval.

## Relevant engineering skill only

For authorized implementation read [rc-implement](.agents/skills/rc-implement/SKILL.md);
for defect reproduction [rc-regression](.agents/skills/rc-regression/SKILL.md).
Design/impact/review tasks select their corresponding skill from
[engineering skills](AI/engineering-skills.md). Reviews are read-only; a self-check
is not independent review. Skills do not authorize broader changes or releases.

## Check entry points (repository root)

Docs: `python3 scripts/verify-docs.py`; instruction/provenance changes also run
`python3 scripts/verify-engineering-skills.py` and the tests in [testing](AI/testing.md).
Go checks run from `backend/`; full commands/prerequisites are in that same doc.
Use [local setup](backend/README.md) when a running service is actually needed.
