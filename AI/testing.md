# Testing strategy

Testing is used as a progress gate for vibe-coded development.

Every roadmap phase has a small observable outcome and tests that prove it.

## Test layers

### 1. Unit tests

Use Go's standard `testing` package.

Best targets:
- setup JSON validation;
- visibility decisions;
- friendship state rules;
- admin email allowlist parsing/checking;
- request validation;
- pagination/cursor helpers if introduced.

Prefer table-driven tests.

### 2. Repository integration tests

Repository SQL must be tested against real PostgreSQL.

Reason: this project depends on PostgreSQL-specific behavior:
- JSONB;
- UUID;
- expression indexes;
- constraints;
- foreign keys;
- `TIMESTAMPTZ`.

Do not rely only on SQL mocks.

Recommended implementation:
- run PostgreSQL in Docker;
- use `testcontainers-go` or a dedicated isolated test database;
- apply real migrations before repository tests;
- clean data between tests.

The exact choice can be made in milestone 1/2. Favor the option that makes `go test ./...` repeatable for a developer with Docker running.

### 3. HTTP tests

Use `httptest`.

Test:
- status codes;
- authentication requirements;
- authorization;
- malformed request handling;
- response shape for critical endpoints.

Do not require a real Google login for most handler tests. Wrap identity/session resolution behind a narrow boundary that tests can substitute.

### 4. End-to-end smoke tests

Add only after core API is implemented.

Minimum automated/manual smoke path:
1. login/test session;
2. list chassis catalog;
3. create setup;
4. read own setup;
5. make it public;
6. find it through public search;
7. create second user;
8. send/accept friendship;
9. switch setup to friends;
10. friend can read it;
11. unrelated user cannot;
12. switch to private;
13. only owner can read it.

## Mandatory regression tests

### Visibility matrix

For each visibility:

| Caller | public | friends | private |
|---|---:|---:|---:|
| owner | allow | allow | allow |
| accepted friend | allow | allow | deny |
| unrelated authenticated user | allow | deny | deny |

This logic must have table-driven tests.

### Friendship uniqueness

Test:
- A -> B pending succeeds;
- another A -> B fails;
- B -> A while A -> B exists fails;
- self-request fails;
- only B can accept A -> B.

### Chassis catalog

Test:
- ordinary user cannot mutate catalog;
- admin can create brand/model;
- inactive model does not appear in regular selectable catalog;
- an existing setup referencing an inactive model still renders correctly;
- setup can use `chassis_model_id = NULL`.

### Setup JSON

Test:
- `0` degrees survives encode/decode as a real value;
- omitted degree remains omitted;
- valid schema v1 persists and loads;
- unsupported schema version is handled explicitly.

## Commands

The final repository should converge on simple commands such as:

```bash
go test ./...
go test -race ./...
```

Later add a `Makefile` or task runner only if it meaningfully simplifies repeatable commands.
