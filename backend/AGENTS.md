# Backend-specific Codex instructions

Applies under `backend/`. Start with the root [task router](../AGENTS.md) and
[workflow](../AI/workflow.md); load only the rows relevant to the change.

| Change | Canonical context |
| --- | --- |
| Go handlers/services/wiring | [Architecture and package boundaries](../AI/architecture.md#package-boundaries), [development](../AI/development.md#coding-rules) |
| Auth/admin/media access | [Security](../AI/architecture.md#authentication-and-security), [private media](../AI/architecture.md#private-media-boundary) |
| JSON/transport | [API](../AI/api.md), [OpenAPI](api/openapi.yaml), affected typed models/tests |
| SQL/atomicity/migrations | [Database](../AI/database.md), actual repositories and [migrations](migrations) |
| Checks | [Testing](../AI/testing.md#commands); run Go commands from this directory |

Keep handlers free of SQL, repositories explicit pgx, and schema-v1 data typed
with nullable numbers preserving zero. These are reminders, not parallel specs;
the linked owners define the full rules. Real PostgreSQL tests remain mandatory
for persistence behavior. No ORM, application-behavior changes outside approved
scope, merge, deployment or live-system access without explicit authorization.
