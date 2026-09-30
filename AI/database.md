# Database design

Database: PostgreSQL.

Use UUID primary keys and `TIMESTAMPTZ`.

## 1. users

Purpose: application users authenticated through Google.

```text
users
-----
id              UUID PK
google_subject  TEXT UNIQUE NOT NULL
email           TEXT UNIQUE NOT NULL
nickname        TEXT UNIQUE NOT NULL
avatar_url      TEXT NULL
created_at      TIMESTAMPTZ NOT NULL
updated_at      TIMESTAMPTZ NOT NULL
```

Rules:
- `google_subject` is the stable external identity key.
- normalize email before persistence;
- nickname uniqueness should be treated case-insensitively at the application/database boundary;
- email uniqueness should also be case-insensitive;
- admin status is NOT stored here.

## 2. friendships

```text
friendships
-----------
id              UUID PK
requester_id    UUID NOT NULL FK -> users.id
addressee_id    UUID NOT NULL FK -> users.id
status          TEXT NOT NULL
created_at      TIMESTAMPTZ NOT NULL
updated_at      TIMESTAMPTZ NOT NULL
```

Allowed status for POC:
- `pending`;
- `accepted`.

Rejecting a pending request may delete it. Removing a friendship deletes the accepted row.

Constraints:
- requester cannot equal addressee;
- there can be only one friendship/request between the same unordered pair of users.

Because `(A,B)` and `(B,A)` must not coexist, implement a PostgreSQL unique expression index using the UUID pair in deterministic order.

Deleting a user should remove related friendship rows.

## 3. chassis_brands

Only admins create or modify catalog entries.

```text
chassis_brands
--------------
id          UUID PK
name        TEXT NOT NULL
is_active   BOOLEAN NOT NULL DEFAULT TRUE
created_at  TIMESTAMPTZ NOT NULL
updated_at  TIMESTAMPTZ NOT NULL
```

Rules:
- brand name unique case-insensitively;
- do not physically delete catalog entries used by setups;
- inactive brands cannot be selected for new setups;
- existing setups keep displaying inactive historical catalog values.

## 4. chassis_models

```text
chassis_models
--------------
id          UUID PK
brand_id    UUID NOT NULL FK -> chassis_brands.id
name        TEXT NOT NULL
is_active   BOOLEAN NOT NULL DEFAULT TRUE
created_at  TIMESTAMPTZ NOT NULL
updated_at  TIMESTAMPTZ NOT NULL
```

Rules:
- model name unique case-insensitively within a brand;
- inactive models cannot be selected for new setups;
- brand relation uses `ON DELETE RESTRICT`.

## 5. setups

```text
setups
------
id                  UUID PK
owner_id            UUID NOT NULL FK -> users.id
chassis_model_id    UUID NULL FK -> chassis_models.id
title               TEXT NOT NULL
visibility          TEXT NOT NULL
data                JSONB NOT NULL
notes               TEXT NULL
schema_version      INTEGER NOT NULL DEFAULT 1
created_at          TIMESTAMPTZ NOT NULL
updated_at          TIMESTAMPTZ NOT NULL
```

Allowed visibility:
- `public`;
- `friends`;
- `private`.

Meaning of nullable `chassis_model_id`:

```text
NULL = custom chassis, unlisted chassis, or chassis intentionally not specified.
```

The POC intentionally does not distinguish those states.

Rules:
- `schema_version >= 1`;
- initial setup schema version is `1`;
- deleting a user deletes their setups;
- catalog entries should normally be disabled rather than deleted;
- if a chassis model is ever physically deleted, `chassis_model_id` may be set to NULL rather than deleting the setup.

## Setup JSON schema v1

The JSONB document is structured data, not an arbitrary JSON bag.

Conceptual shape:

```json
{
  "suspension": {
    "front": {
      "camber_deg": -5.5,
      "caster_deg": 7.0,
      "toe_deg": 1.0,
      "link_lengths": [
        {
          "name": "camber_link",
          "length_mm": 42.5
        }
      ]
    },
    "rear": {
      "camber_deg": -2.0,
      "caster_deg": null,
      "toe_deg": 2.5,
      "link_lengths": []
    }
  },
  "shocks": {
    "front": {
      "manufacturer": "Yokomo",
      "model": "Big Bore",
      "spring": {
        "manufacturer": "Yokomo",
        "color": "Purple"
      },
      "oil_cst": 250
    },
    "rear": {
      "manufacturer": "Yokomo",
      "model": "Big Bore",
      "spring": {
        "manufacturer": "Yokomo",
        "color": "Blue"
      },
      "oil_cst": 300
    }
  },
  "electronics": {
    "motor": "Acuvance Fledge 10.5T",
    "esc": "Acuvance Xarvis XX",
    "servo": "Reve D RS-ST",
    "gyro": "Yokomo DP-302 V4",
    "radio": "Futaba T10PX"
  }
}
```

Most technical fields should be optional. In Go, optional numeric fields must not use a plain numeric zero value because zero degrees is a legitimate setup value.

For example:

```go
type AxleSuspension struct {
    CamberDeg   *float64     `json:"camber_deg,omitempty"`
    CasterDeg   *float64     `json:"caster_deg,omitempty"`
    ToeDeg      *float64     `json:"toe_deg,omitempty"`
    LinkLengths []LinkLength `json:"link_lengths,omitempty"`
}
```

This preserves the difference between:
- omitted/unknown;
- exactly `0`.

The same principle applies to oil viscosity and other numeric data.

## Initial indexes

Create only useful POC indexes:

- users: case-insensitive unique email;
- users: case-insensitive unique nickname;
- friendships: requester;
- friendships: addressee;
- friendships: unique unordered user pair;
- chassis_models: brand;
- setups: owner;
- setups: chassis_model;
- setups: visibility;
- setups: `(visibility, created_at DESC)` for public browsing/search.

Do not add a general JSONB GIN index until a real JSON-field search requirement exists.

## Transactions

Use transactions for operations that must remain atomic, especially:
- friendship acceptance if multiple rows/checks are involved;
- catalog changes with dependent validation if needed.

Simple single-row setup CRUD does not need an explicit transaction unless it performs multiple dependent writes.
