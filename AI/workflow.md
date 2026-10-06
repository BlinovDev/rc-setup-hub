# Feature delivery workflow

This document defines the intended lifecycle for AI-assisted feature work. GitHub is the durable source of task state; chat history is not.

## Source of truth

For a new task, reconstruct context from:
1. `AGENTS.md`;
2. `AI/product.md`;
3. `AI/decisions.md`;
4. relevant architecture/API/testing docs;
5. the GitHub Issue/PR for the feature;
6. the current code.

Do not rely on a previous ChatGPT conversation being available.

## Feature lifecycle

```text
idea
 -> validated specification
 -> GitHub Issue
 -> implementation branch / PR
 -> automated checks
 -> staging deployment
 -> QA
 -> approved merge to main
 -> production deployment
 -> smoke verification
```

## Specification gate

Before implementation, turn a feature request into explicit acceptance criteria. Identify:
- user-visible behavior;
- backend/API changes;
- frontend changes;
- authorization/business-rule impact;
- data/migration impact;
- test cases;
- explicit out-of-scope behavior;
- unresolved questions.

Do not guess through a material ambiguity. Ask before implementation when different answers would change business behavior, authorization, stored data, or API compatibility.

## Implementation rules

- Work on a feature branch and propose changes through a PR.
- Keep scope tied to the approved acceptance criteria.
- Update automated tests for behavior changes.
- Update `AI/product.md` when product/business behavior changes.
- Update `AI/decisions.md` when a durable architectural/product decision changes.
- Update API/architecture/deployment docs when their contracts change.
- Never commit credentials or production secrets.
- Never deploy a feature branch directly as production.

For cross-repository features, keep the backend and frontend PRs linked in their descriptions and state the required merge/deploy order when order matters.

## Staging and QA

A successful implementation is not production approval.

The intended automation should deploy validated feature work to a staging environment with isolated configuration and data. QA feedback belongs on the Issue/PR so a fresh agent can recover it without chat history.

QA failure:
- preserve the existing feature scope;
- reproduce and fix the reported problem;
- add/update a regression test where practical;
- rerun checks;
- redeploy staging for another QA pass.

QA pass means the feature is eligible for the production approval gate; it does not itself authorize an agent to bypass that gate.

## Production

Production changes come from `main` after the configured approval/merge gate.

Deployment automation should:
- use production-specific secrets/configuration;
- apply migrations safely when required;
- deploy the application;
- run health/smoke checks;
- report failure clearly.

Staging and production may share one physical server, but must use separate runtime configuration and separate databases. A staging process must never point at the production database.

## Definition of done

A feature is done only when:
- acceptance criteria are satisfied;
- relevant automated checks pass;
- QA has passed when required;
- documentation reflects changed product/contract behavior;
- the approved change is merged to `main`;
- production deployment and smoke checks succeed.
