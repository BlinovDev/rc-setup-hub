# API contract — POC direction

Prefix JSON endpoints with:

```text
/api/v1
```

The exact response envelope can be finalized during implementation, but keep it consistent.

Unless explicitly stated otherwise, application endpoints require authentication.

## Health

```text
GET /health
```

Returns service/database health information suitable for local/deployment checks.

## Authentication

```text
GET  /auth/google
GET  /auth/google/callback
POST /api/v1/auth/logout
GET  /api/v1/me
```

Expected behavior:
- login begins on backend;
- callback upserts user;
- backend creates its own secure session;
- logout clears session;
- `GET /api/v1/me` returns authenticated profile.

First-time login needs a nickname policy. Recommended POC behavior:
- create a temporary/generated nickname or derive a safe initial candidate;
- require user to set/change it if uniqueness cannot be satisfied cleanly.

Do not use email as the public nickname.

## Profile

```text
PATCH /api/v1/me
```

POC editable field:
- nickname.

## Users / friend discovery

```text
GET /api/v1/users/search?q=<nickname>
```

Search by nickname only for the POC. Do not expose arbitrary user email search.

Response should contain only public profile fields required to add a friend:
- user id;
- nickname;
- avatar URL if available.

## Friendships

```text
GET    /api/v1/friendships
POST   /api/v1/friendships
POST   /api/v1/friendships/{id}/accept
DELETE /api/v1/friendships/{id}
```

`POST /friendships` creates a pending request.

`DELETE` semantics:
- pending incoming request -> reject;
- pending outgoing request -> cancel;
- accepted friendship -> remove.

Rules:
- cannot add self;
- cannot create reverse duplicate;
- only addressee can accept;
- both parties can remove an accepted friendship.

## Chassis catalog — client read API

```text
GET /api/v1/chassis/brands
GET /api/v1/chassis/brands/{brand_id}/models
```

Return active values only to the regular client.

The client itself can display a synthetic `Custom / not listed` option. It must not create a fake catalog database row.

When custom/unlisted is selected, send:

```json
{
  "chassis_model_id": null
}
```

## Setups

```text
POST   /api/v1/setups
GET    /api/v1/setups/{id}
PATCH  /api/v1/setups/{id}
DELETE /api/v1/setups/{id}

GET    /api/v1/me/setups
GET    /api/v1/setups/search
GET    /api/v1/users/{user_id}/setups
```

### Create/update fields

```json
{
  "title": "RD2.0 carpet setup",
  "chassis_model_id": "uuid-or-null",
  "visibility": "public",
  "notes": "Important tuning notes",
  "data": {
    "suspension": {},
    "shocks": {},
    "electronics": {}
  }
}
```

`schema_version` is controlled by the backend, not supplied as an arbitrary client value for normal create requests.

### Visibility

Access to a setup detail:

- owner: always allowed;
- `public`: any authenticated app user;
- `friends`: owner or accepted friend;
- `private`: owner only.

### Public search

`GET /api/v1/setups/search` searches **public setups only**.

Initial filters:

```text
q
brand_id
model_id
limit
cursor
```

Initial `q` may search:
- setup title;
- owner nickname.

Do not search technical JSONB fields during the initial POC.

### User setup list

`GET /api/v1/users/{user_id}/setups` returns only rows visible to the caller:
- public;
- friends if accepted friendship exists;
- all when caller is the owner.

## Sharing

The POC does not need a separate share table.

A setup has a stable detail URL based on its UUID. Sharing means sending that URL. Access is still enforced by backend visibility rules.

## Admin panel

Admin is server-rendered HTML, not JSON-first.

Routes:

```text
GET  /admin
GET  /admin/users

GET  /admin/chassis/brands
POST /admin/chassis/brands
POST /admin/chassis/brands/{id}/update
POST /admin/chassis/brands/{id}/disable
POST /admin/chassis/brands/{id}/enable

GET  /admin/chassis/models
POST /admin/chassis/models
POST /admin/chassis/models/{id}/update
POST /admin/chassis/models/{id}/disable
POST /admin/chassis/models/{id}/enable
```

Dashboard minimum statistics:
- total users;
- total setups;
- public setups.

Every `/admin` route requires:
1. authenticated user;
2. authenticated email present in `ADMIN_EMAILS`.

Admin form actions require CSRF protection.
