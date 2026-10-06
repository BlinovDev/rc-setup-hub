# Product and business rules

This document is the compact current product truth for agents starting from a fresh chat or task. It describes user-visible behavior and invariants, not implementation history. When behavior changes, update this file in the same PR.

## Product

RC Setup Hub is a web application for RC drift drivers to store, find, and share car setups. The POC targets a local drift community but should remain usable by a wider community without adding unrelated RC disciplines.

The user-facing application is a React frontend backed by a Go JSON API and PostgreSQL. Google is the only authentication provider. The backend also owns a small server-rendered admin area.

## Users and authentication

- A Google identity maps to one application user by stable Google subject.
- An application user has an email, unique nickname and optional avatar.
- Browser authentication is a backend-owned HttpOnly session cookie.
- Frontend JavaScript never stores Google tokens or application session tokens.
- Successful login returns the browser to the trusted configured frontend URL.

## Chassis catalog

- Chassis brands and models are managed by admins.
- Regular users can select active catalog entries but cannot create catalog entries.
- A setup may have no chassis model. This represents Custom/unlisted/unspecified and is not a synthetic database catalog row.
- Existing setups retain historical chassis display data even when catalog entries become inactive.

## Setups

Authenticated users can create, read, update and delete their own setups.

A setup contains:
- title;
- owner;
- optional chassis model;
- visibility;
- structured technical data;
- optional notes;
- schema version;
- timestamps.

Technical data schema v1 covers suspension, shocks/springs and electronics. Optional numeric values preserve the distinction between omitted and explicit zero.

Only the owner may modify or delete a setup.

## Visibility

Visibility values are:
- `public`;
- `friends`;
- `private`.

Access rules:
- owner: can read all own setups;
- accepted friend: can read another user's public and friends setups;
- other authenticated user: can read another user's public setups only;
- pending friendship does not grant friends visibility;
- removing an accepted friendship removes friends-only access immediately;
- inaccessible setup detail must not reveal whether the setup exists or is private.

Public discovery/search returns public setups only. User profile setup lists are filtered by backend authorization for the current viewer.

## Friendships

- Users find other users by nickname, not email.
- A friendship starts as a pending request.
- Only the addressee can accept it.
- Requester can cancel; addressee can reject; either participant can remove an accepted friendship.
- There can be only one relationship for an unordered pair of users.
- A user cannot friend themselves.

## Public profiles

Direct user profiles expose safe public identity only: application user ID, nickname and optional avatar. Email and Google/provider identity are not public profile data.

## Admin

Admin UI is part of the Go backend, not the React SPA.

Admin authorization comes from the configured email allowlist, not a database role. Admin functionality includes chassis catalog management and minimal operational user/statistics views.

## Deliberate non-goals unless explicitly requested

Do not add by assumption:
- comments, likes, ratings or favorites;
- chat or notifications;
- image uploads;
- setup version history;
- teams/groups;
- multiple RC disciplines;
- user-created chassis catalog entries;
- normalized electronics/shock catalogs;
- native-mobile-specific backend behavior;
- offline synchronization.

## Cross-repository contract

The backend OpenAPI document is the machine-readable API contract. The frontend keeps a committed copy and generates transport types from it.

When a feature changes the API:
1. backend implementation, tests and OpenAPI change together;
2. frontend copies the updated OpenAPI contract;
3. generated frontend types are regenerated;
4. frontend behavior and tests are updated.

Do not make frontend and backend infer incompatible versions of a contract.
