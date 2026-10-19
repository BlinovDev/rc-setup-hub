# AI Engineering Skills Foundation — Phase 1

This repository owns six instruction-only Codex skills under `.agents/skills/`.
They supplement the existing AGENTS.md and AI documents; they do not replace any
project decision, test gate, authorization or completion rule. No application,
dependency, API, schema, CI workflow or infrastructure change is part of Phase 1.

## Routing and role boundaries

| Skill | Intended request | May edit? |
| --- | --- | --- |
| `$rc-implement` | Implement an approved feature/fix with the smallest complete change | Only within the authorized task |
| `$rc-architect` | Explain ownership/boundaries and compare design choices | No; report only |
| `$rc-regression` | Reproduce a defect and capture failing-before/passing-after evidence | Tests/fix within an authorized implementation task; a test-plan request alone stays read-only |
| `$rc-blast-radius` | Find downstream impact and challenge the key safety assumption | No; evidence/report only |
| `$rc-review` | Independently assess correctness, security and regressions | No; report only |
| `$rc-thermo-review` | Strict architecture/maintainability quality audit | No; report only |

Read applicable AGENTS.md files and the already-required AI context first, then
load only the relevant SKILL.md. Names/descriptions support selection; each body
contains its own scope and repository-specific invariants. These skills are not a
mandatory six-stage ceremony for every change. Implementation uses Ponytail's
reuse/simplicity principles; review examines evidence independently. Use a fresh
session for independent review; label an implementation session's own pass as a
self-check. No model diversity or independence is implied by a skill name.

None of these skills grants permission to merge, deploy, modify live systems,
post remote comments/approvals, or perform broad refactoring. Report redesign
ideas separately instead of turning a review into implementation. They use file
inspection, search and permitted local checks available in the current harness;
there is no dependency on a plugin, MCP server, background agent or special tool.

## Upstream analysis and adaptations

These are curated adaptations, not upstream plugin installations. Exact input
paths, commit IDs and SHA-256 hashes are recorded in [skills-sources.json](skills-sources.json).
All three selected sources are MIT-licensed at these revisions. Full original
copyright/permission notices are retained in [skill-licenses/](skill-licenses/).
The adapted skill text is distributed under MIT; this does not change the license
of application code. Local scripts/docs are maintained here with the foundation.

| Source and pinned revision | Adopted | Changed or deliberately excluded |
| --- | --- | --- |
| [Ponytail](https://github.com/DietrichGebert/ponytail/tree/09c434ab73ca43761c5d8b0c5d341926c2fba710), `09c434ab73ca43761c5d8b0c5d341926c2fba710` | Reuse before new code; smallest complete fix; trace callers; evidence-backed, practical review | No sticky session mode, levels, slash-command syntax or universal final-response boilerplate. Simplicity never cuts required safety/accessibility/tests. |
| [pstack](https://github.com/backnotprop/pstack/tree/6f131f137de64fdaf928278c0fd9ecec520ccbee), `6f131f137de64fdaf928278c0fd9ecec520ccbee` | Architect ownership/information hiding; regression red/green; blast-radius safety facts; interrogate evidence and severity methodology | No arena, companion-skill dependency, model table, tool-specific Task calls, background orchestration, scaffold commits or automatic implementation after analysis. Expensive required integration tests cannot be waived. |
| [Cursor team kit Thermo-Nuclear](https://github.com/cursor/plugins/blob/d457567332aa6989e753ea289f30ea2bfbc80bc0/cursor-team-kit/skills/thermo-nuclear-code-quality-review/SKILL.md), `d457567332aa6989e753ea289f30ea2bfbc80bc0` | Search for structural simplification, canonical ownership, type/boundary cleanliness and unnecessary complexity | Read-only architecture report. A 1,000-line threshold prompts cohesion analysis, not a mechanical blocker. No permission to restructure the codebase. No Cursor-only `disable-model-invocation` or approval authority. |

The architecture skill does not prescribe a new architecture or import upstream
preferences that conflict with accepted project contracts. The regression skill
reports missing prerequisites instead of treating an environment failure as a
reproduced bug. Review findings require concrete evidence, reachable impact and a
bounded remedy; strictness is not a quota or a license for speculative rewrites.

## Repeatable verification

Run from this repository root with Python 3.9+ (standard library only):

```sh
python3 scripts/verify-engineering-skills.py
python3 -m unittest discover -s scripts -p 'test_engineering_skills.py' -v
git diff --check
```

The checker accepts a deliberately small valid-YAML subset: `name`, `description`
and `license` use JSON double-quoted scalar values. It checks inventory, metadata,
local links, pinned source records and exact license bytes without network access.
The regression suite rejects missing skills, unsupported frontmatter, broken links,
changed notices and escaping paths. This proves packaging, **not** agent discovery,
invocation, policy enforcement or review quality. It does not claim to validate all
possible Agent Skills syntax. License hashes are preservation checks, not a legal
or upstream authenticity certification; maintainers must inspect pinned inputs.

### Fresh Codex Cloud discovery and invocation smoke

Run this procedure separately for each repository on a Cloud task checked out at
the exact PR/head revision. Restart the task after adding skills; an existing
session may have a cached catalog. Do not rely on the other repository checkout,
user-global skills or a parent workspace. Record repository, head SHA, date,
Cloud task URL/transcript and actual tool results in the PR.

1. Before manually locating skill files, ask: "List the repository skills already
   present in your available skill catalog, including names, descriptions and
   paths. Do not infer catalog discovery from AGENTS.md or a directory scan. If
   the six rc-* skills are absent, report NOT DISCOVERED and stop."
2. In fresh sessions, explicitly invoke each skill with the corresponding prompt
   below. Do not replace `$name` with an instruction to read a hard-coded path:
   that would test manual file access rather than discovery. Check the tool trace
   for the actual selected `.agents/skills/<name>/SKILL.md` read and that session's
   correct repository context. An assistant saying "used" alone is insufficient.
3. Verify the requested artifact, role limits and citations. Check `git status
   --short` before/after each read-only session. No source/fixture/generated edits,
   live system access, deployment, merge or remote write is allowed. If supported,
   use a read-only sandbox; prose restrictions are not tool-level enforcement.
4. Mark discovery and each invocation PASS/FAIL/NOT RUN individually. An absent
   tool trace or unavailable Cloud environment is NOT VERIFIED, never PASS. Keep
   the transcript and list any additional user/global instructions affecting it.

| Invoke in a fresh task | Expected evidence |
| --- | --- |
| `Use $rc-implement. Plan only: explain the smallest complete fix for an optional numeric field losing explicit zero. Do not edit files.` | Loads implementation skill; finds the existing owner/helper and project tests; respects plan-only scope; no new abstraction by default |
| `Use $rc-architect. Analyze ownership between transport, domain/server state and persistence for setup photos. Do not implement.` | Loads architecture skill; actual file evidence, existing boundaries, bounded design analysis; no scaffolds |
| `Use $rc-regression. Give a test plan only for empty numeric input versus explicit zero; report missing prerequisites. Do not edit or run a fix.` | Loads regression skill; behavior-focused red/green plan; does not claim it ran or waive mandatory checks |
| `Use $rc-blast-radius. Analyze the current API contract consumers and the safety assumption behind changing a nullable setup value. No changes.` | Loads blast-radius skill; downstream boundaries and evidence levels; unexecuted assumptions explicitly unproven |
| `Use $rc-review. Review the foundation PR diff independently; report findings only, no fixes or remote comments.` | Loads review skill; confirms base/head, inspects connected context, grounded findings or explicit none, no edits/approval |
| `Use $rc-thermo-review. Audit architecture quality of the foundation PR diff. Report only; do not refactor.` | Loads strict architecture skill; supported complexity/boundary findings, optional redesign separate; no automatic line-count rejection |

For the numeric examples, frontend examines its committed contract and client
serialization; backend examines its own typed parsing/persistence. No test prompt
authorizes reading the other repository's source. Repeat implicit selection with
an ordinary equivalent request if desired, recording it separately: semantic
routing is model/harness-dependent and not established by explicit invocation.
These read-only smokes test selection and role boundaries; actual failing-before/
passing-after implementation competence needs a separately scoped disposable bug
fixture or later real authorized task, not an unreviewed application change here.

### Verification status and Cloud limitations

Phase 1 can verify files, links, provenance, license preservation and checker
behavior locally. **Codex Cloud discovery, explicit/implicit invocation, independent
review behavior and sandbox enforcement are NOT VERIFIED by those checks.** No
Cloud task execution is available from this working session. The procedure above
is the outstanding acceptance check in a fresh Cloud task; record real evidence
before claiming it passes. No changes to Cloud environment setup are installed.

Cloud shell/network, Docker/PostgreSQL, native photo decoders, ARM64 execution,
Node/browser installations, physical-device QA and GitHub permissions depend on
the actual task environment. Skills do not provision or guarantee them. Existing
AI/testing.md and CI requirements remain unchanged; report unavailable checks.
No hooks, automatic skill chains, named external models or background orchestration
are bundled or claimed to work.

Official guidance checked for this foundation:
[repository skills and progressive disclosure](https://developers.openai.com/blog/skills-agents-sdk)
and [explicit invocation/evaluation](https://developers.openai.com/blog/eval-skills).
These establish the packaging approach, not verification of this account's Cloud
runtime. Recheck current official guidance when repeating the Cloud smoke.

## Updating the vendored adaptations

1. Open a narrowly scoped maintenance branch in this repository. Read its current
   AGENTS.md/AI instructions first; inspect each repository independently.
2. Fetch the upstream repository into a disposable directory; resolve a full
   commit SHA. Inspect the input paths listed in skills-sources.json with
   `git show <old-sha>:<path>` and `git show <new-sha>:<path>`. Review changed license
   scope/notices before adapting content; do not assume an unchanged license.
3. Adopt only useful workflow changes. Preserve local boundaries, decisions,
   required tests and read-only reviews. Do not copy tool/model settings, hooks,
   executable installers or automatic merge/deploy/refactor instructions.
4. Update selected input paths/hashes, revision, exact license copy/hash and the
   source table together. Hash original upstream bytes (not the adapted text);
   for example `git show <sha>:<path> | shasum -a 256`. Retain applicable notices.
5. Run structural/regression checks, repository formatting requirements and fresh
   Cloud smoke cases. Review a real scoped example for behavioral regressions;
   do not convert static validation into a claim of runtime correctness.
6. Create a PR listing adopted/rejected changes, verification results and unknowns.
   If changes should apply to both repositories, adapt and review in a separate
   PR there too; there is no automatic sync or dependency between checkouts.
   Do not merge, deploy or broaden the change as part of a skill update.
