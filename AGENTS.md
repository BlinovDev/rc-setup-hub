# RC Setup Hub — Codex instructions

This repository is built incrementally. Keep changes small, testable, and easy to review.

## Read before changing code

Start with `AI/main.md` and `AI/product.md`. For feature delivery or work that may reach staging/production, also read `AI/workflow.md`. Then read only the documents relevant to the task:
- product/business rules: `AI/product.md`
- feature delivery lifecycle: `AI/workflow.md`
- architecture: `AI/architecture.md`
- database: `AI/database.md`
- API: `AI/api.md`
- development rules: `AI/development.md`
- testing: `AI/testing.md`
- implementation sequence: `AI/roadmap.md`
- accepted decisions: `AI/decisions.md`

Do not silently change accepted decisions or business rules. If a task conflicts with `AI/decisions.md` or materially changes `AI/product.md`, make that change explicit in the specification/PR instead of guessing.

For new feature requests, treat GitHub Issue/PR state and repository docs as durable context. Do not rely on previous chat history being available.

## Development rules

- For roadmap work, implement one roadmap step at a time.
- For post-roadmap feature work, implement only the approved acceptance criteria.
- Do not add features opportunistically.
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
- Update `AI/product.md` in the same PR when user-visible behavior or a business rule changes.
- For cross-repository API changes, update backend OpenAPI first and keep the frontend PR linked.

## Completion rule

A task is complete only when:
1. implementation matches its acceptance criteria;
2. relevant tests pass;
3. no unrelated behavior was changed;
4. documentation is updated when product behavior or a contract changed;
5. staging/QA/production gates in `AI/workflow.md` are respected when applicable.

When asked to implement a roadmap phase, first state which checklist items you will complete, then implement them and report the tests executed.
