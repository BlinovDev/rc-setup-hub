# POC implementation roadmap

Implement these phases in order. Do not start the next phase until the current checkpoint is green.

---

## Phase 0 — repository bootstrap

Goal: create the repository and agent context without application behavior.

Tasks:
- create root documentation;
- initialize Git;
- initialize `backend/go.mod`;
- create target directories;
- verify Codex reads `AGENTS.md` and `AI/main.md`.

Checkpoint:
- `go env GOMOD` points to `backend/go.mod`;
- Codex can accurately summarize the accepted architecture without editing files.

No application code is required.

---

## Phase 1 — runnable Go service

Goal: the backend starts and exposes health.

Tasks:
- add `cmd/server/main.go`;
- config package for `HTTP_ADDR`;
- chi router;
- request logging;
- panic recovery;
- `GET /health`;
- graceful shutdown;
- basic unit/HTTP tests.

Checkpoint:

```bash
go test ./...
go run ./cmd/server
curl http://localhost:8080/health
```

Expected: HTTP 200.

Do not add authentication or database domain tables yet.

---

## Phase 2 — PostgreSQL + migrations

Goal: backend connects to PostgreSQL and owns the approved schema.

Tasks:
- local PostgreSQL development setup;
- `DATABASE_URL`;
- pgx pool;
- goose migration runner/workflow;
- migrations for:
  - users;
  - friendships;
  - chassis_brands;
  - chassis_models;
  - setups;
  - constraints and initial indexes;
- database-aware health check;
- repository integration-test harness.

Tests:
- migrations apply cleanly from empty database;
- uniqueness/check constraints work;
- nullable chassis model works;
- reverse duplicate friendship pair is rejected;
- schema version default is 1.

Checkpoint:
- clean DB can migrate from zero;
- `go test ./...` passes with local Docker available.

---

## Phase 3 — user model and authentication foundation

Goal: a Google-authenticated person maps to one application user and receives a backend session.

Tasks:
- Google OAuth/OIDC configuration;
- login endpoint;
- callback;
- user upsert by `google_subject`;
- secure backend session;
- logout;
- `GET /api/v1/me`;
- nickname initialization/update strategy;
- auth middleware.

Tests:
- user upsert does not duplicate same Google subject;
- email update does not change identity;
- unauthenticated `/me` -> 401;
- authenticated `/me` -> current user;
- logout invalidates local session behavior.

Checkpoint:
- developer can sign in locally with Google and call `/api/v1/me`.

---

## Phase 4 — admin authorization + shell

Goal: admin is part of backend and protected by environment allowlist.

Tasks:
- parse `ADMIN_EMAILS`;
- admin middleware;
- `/admin`;
- minimal HTML layout/templates;
- 403 for authenticated non-admin;
- CSRF protection for future forms.

Tests:
- allowlisted user allowed;
- normal authenticated user forbidden;
- unauthenticated user redirected/rejected according to chosen admin login flow;
- malformed/empty allowlist handled safely.

Checkpoint:
- allowlisted email can open admin page;
- ordinary user cannot.

---

## Phase 5 — chassis catalog

Goal: admin controls selectable chassis data.

Tasks:
- brand repository/service;
- model repository/service;
- admin pages/forms:
  - list brands;
  - create/edit;
  - enable/disable;
  - list models;
  - create/edit;
  - enable/disable;
- read-only client API:
  - list active brands;
  - list active models for a brand.

Rules:
- users cannot create catalog items;
- no database `Custom` record;
- custom/unlisted setup uses `chassis_model_id = NULL`.

Tests:
- case-insensitive duplicates rejected;
- inactive values hidden from selection API;
- normal users cannot mutate;
- admin mutations work.

Checkpoint:
- admin can create `Yokomo -> RD2.0`;
- client API returns it.

---

## Phase 6 — setup JSON schema v1 + CRUD

Goal: authenticated user can own setups.

Tasks:
- explicit Go structs for schema v1;
- optional numeric fields represented without losing valid zero;
- setup validation;
- create;
- read;
- update;
- delete;
- own setup list;
- chassis model validation;
- backend sets `schema_version = 1`.

Tests:
- JSON round-trip;
- omitted vs zero numeric values;
- owner derived from authenticated user;
- user cannot update/delete another user's setup;
- unknown/inactive chassis model rejected for new setup;
- NULL chassis allowed.

Checkpoint:
- a logged-in user can create/read/update/delete a realistic drift setup.

---

## Phase 7 — setup visibility

Goal: access control is correct before social/search features build on it.

Tasks:
- central visibility policy/service;
- public access;
- friends-only access using accepted friendship;
- private access;
- user setup listing filtered by caller permissions.

Tests:
- complete visibility matrix from `AI/testing.md`;
- pending friendship does not grant access;
- removed friendship immediately stops granting friends-only access.

Checkpoint:
- automated visibility matrix is green.

---

## Phase 8 — friendships

Goal: users can form accepted friend relationships.

Tasks:
- nickname user search;
- create request;
- incoming/outgoing/accepted list;
- accept;
- reject/cancel/remove;
- concurrency-safe duplicate handling.

Tests:
- unordered-pair uniqueness;
- self-add rejected;
- only addressee accepts;
- reverse request cannot duplicate;
- friendship removal affects setup visibility.

Checkpoint:
- two test users can complete request -> accept -> view friends-only setup.

---

## Phase 9 — public setup search

Goal: find useful public setups without searching JSONB internals.

Tasks:
- `/api/v1/setups/search`;
- title search;
- owner nickname search;
- brand filter;
- model filter;
- pagination;
- sort newest-first initially.

Rules:
- public setups only;
- do not expose friends/private setups through search;
- do not add JSONB GIN index yet.

Tests:
- private/friends setup never leaks into public search;
- brand/model filtering;
- title/nickname query;
- pagination stable enough for POC.

Checkpoint:
- public setup can be created and found by another user.

---

## Phase 10 — admin statistics/users

Goal: finish the minimum requested admin functionality.

Tasks:
- total users;
- total setups;
- public setups;
- users table with:
  - nickname;
  - email;
  - created timestamp;
  - setup count if cheap/simple.

Tests:
- counts are correct with fixture data;
- admin-only access.

Checkpoint:
- `/admin` gives the requested operational overview.

---

## Phase 11 — client application

Goal: build the actual end-user UX against the stable API.

Recommended POC direction: React web app/PWA. Final client choice can be made before this phase without changing backend domain design.

Minimum screens:
- login;
- own profile;
- own setups;
- create/edit setup;
- public search;
- setup detail/share URL;
- friend search/requests/list;
- another user's visible setups.

Do not start this phase until core backend flows have integration coverage.

---

## Phase 12 — POC hardening/deploy

Goal: make it safe enough for real track users.

Tasks:
- production CORS config;
- secure cookies;
- HTTPS assumptions documented;
- database backup plan;
- migration deploy procedure;
- rate limits for auth/search if needed;
- request size limits;
- error pages/API errors;
- deployment health check;
- smoke test.

Checkpoint:
- clean deploy can migrate DB and complete the E2E smoke path.
