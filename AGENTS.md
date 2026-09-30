# RC Setup Hub — Codex instructions

This repository is built incrementally. Keep changes small, testable, and easy to review.

## Read before changing code

Start with `AI/main.md`. Then read only the documents relevant to the task:
- architecture: `AI/architecture.md`
- database: `AI/database.md`
- API: `AI/api.md`
- development rules: `AI/development.md`
- testing: `AI/testing.md`
- implementation sequence: `AI/roadmap.md`
- accepted decisions: `AI/decisions.md`

Do not silently change accepted decisions. If a task conflicts with `AI/decisions.md`, stop and explain the conflict.

## Development rules

- Implement one roadmap step at a time.
- Do not add features that are not required by the current roadmap step.
- Prefer simple code over generic abstractions.
- Keep `cmd/server` limited to application wiring.
- Domain logic belongs in the corresponding `internal/...` package.
- Database access must not be mixed directly into HTTP handlers.
- Validate API input at the boundary and domain invariants in the domain/service layer.
- Add or update tests with every behavior change.
- Run formatting and the relevant test suite before considering a task complete.
- Never commit secrets or real credentials.
- Do not replace PostgreSQL with another database.
- Do not replace structured Go setup types with arbitrary `map[string]any`.

## Completion rule

A roadmap task is complete only when:
1. implementation matches its acceptance criteria;
2. relevant tests pass;
3. no unrelated behavior was changed;
4. documentation is updated if the contract changed.

When asked to implement a roadmap phase, first state which checklist items you will complete, then implement them and report the tests executed.
