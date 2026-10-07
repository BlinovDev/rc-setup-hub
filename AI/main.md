# Project context entry point

## Product

RC Setup Hub is a POC for a local RC drift community. Users store and share normalized RC car setup data.

The initial goal is not to model every possible RC discipline. Build only what is required for drift POC usage and keep the data model easy to extend later.

## Core POC use cases

1. Sign in with Google.
2. Have a profile containing email and unique nickname.
3. Create a setup.
4. Choose a chassis brand/model from an admin-managed catalog, or leave the chassis model unset for a custom/unlisted setup.
5. Store setup data for suspension, shocks, electronics, and notes.
6. Set setup visibility to `public`, `friends`, or `private`.
7. Search public setups.
8. Add another user as a friend via a request/accept flow.
9. View friends-only setups only when friendship is accepted.
10. Use a minimal backend admin panel to view users/statistics and manage chassis brands/models.

## Non-goals for the POC

Do not add unless a new accepted decision explicitly requires them:

- multiple RC disciplines;
- comments;
- likes;
- favorites;
- setup version history;
- ratings;
- notifications;
- teams/groups;
- chat;
- image uploads;
- normalized electronics catalog;
- normalized shock/spring catalog;
- arbitrary user-created chassis catalog entries;
- MongoDB;
- native mobile-specific backend behavior.

## Data strategy

Use PostgreSQL.

Relational concepts are normalized:
- users;
- friendships;
- chassis brands;
- chassis models;
- setup ownership and visibility.

Technical setup details are stored in a `JSONB` column but validated through explicit Go structs.

Every setup has `schema_version`. Initial value is `1`.

## Client and admin boundary

The user client is a separate React application in its own repository and
communicates with the backend JSON API over exact credentialed CORS. Frontend
requests use credentials: "include" with the backend-managed HttpOnly session
cookie; frontend JavaScript never receives/stores Google/backend auth tokens.
Normal backend startup requires trusted APP_URL; successful Google login creates
the session and redirects there. ALLOWED_ORIGINS is independently configured.

The machine-readable frontend contract is backend/api/openapi.yaml, also served
at GET /openapi.yaml. The frontend copies/downloads it and commits its own
api/openapi.yaml for reproducible TypeScript generation, without assumptions
about checkout locations. Direct user profile lookup exposes only safe public
fields. Setup responses expose owner application UUID and historical chassis
names; inactive selection and visibility/ownership restrictions remain enforced.

The admin panel is part of the Go backend and is rendered by the backend itself.

Admin authorization is not stored as a role in the database. The backend reads a list of allowed admin Google email addresses from the environment.

## Read next

- `AI/architecture.md`
- `AI/database.md`
- `AI/api.md`
- `AI/roadmap.md`

## Deployment

See `AI/deployment.md` for the concrete CI, paired-release, staging, production and recovery contract.
