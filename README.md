# RC Setup Hub

RC Setup Hub is a proof-of-concept application for storing, finding, and sharing setup data for radio-controlled drift cars.

The POC is initially intended for a local RC drift-track community. The backend is a Go application backed by PostgreSQL. A separate client application will consume the backend API. The backend also serves a minimal server-rendered admin panel.

## POC features

- Google-only authentication.
- User profile with email and unique nickname.
- Create, edit, view, and delete RC setups.
- Public, friends-only, and private setup visibility.
- Search public setups.
- Send and accept friend requests.
- Admin-only chassis brand/model catalog.
- Minimal admin statistics and user list.
- PostgreSQL storage with normalized relational data plus structured `JSONB` setup details.
- Explicit `schema_version` on every setup.

## Architecture

```text
Separate client application
        |
        | HTTPS + CORS
        v
Go backend
├── /api/v1/...        JSON API
├── /auth/...          Google authentication
├── /admin/...         server-rendered admin UI
└── PostgreSQL
```

The admin panel is part of the Go backend. It is not a second frontend application.

Admin access is controlled by a comma-separated allowlist of Google email addresses in the backend environment.

## Repository docs

Start with:

- `AGENTS.md` — instructions for coding agents.
- `AI/main.md` — project context entry point.
- `AI/product.md` — current product behavior and business rules.
- `AI/workflow.md` — durable feature/PR/staging/QA/production lifecycle.
- `AI/roadmap.md` — ordered implementation plan and checkpoints.
- `AI/database.md` — accepted database model.
- `AI/api.md` — intended HTTP contract.
- `AI/testing.md` — testing strategy.
- `AI/development.md` — engineering rules.

## Current status

Core backend POC flows are implemented: Google authentication/session handling, profiles, chassis catalog/admin, setup CRUD and visibility, friendships, public setup search, public profiles, and the frontend OpenAPI contract.

The next infrastructure milestone is hardening and deployment, including isolated staging/production configuration, migration/deploy procedures, backups, health/smoke checks, and CI/CD automation.
