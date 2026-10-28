# Shared enduring product rules

This backend document owns shared RC Setup Hub business rules. The separate
[frontend product](https://github.com/BlinovDev/rc-setup-hub-fe/blob/main/AI/product.md)
owns client interaction rules; transport details belong to [API](api.md)/OpenAPI.
Change this file only for an enduring product-rule change, per [workflow](workflow.md).

## Identity, catalog and setups

- RC drift setups only; Google sign-in; stable Google subject identifies a user.
  Users have unique case-insensitive nicknames/emails and optional avatars.
- The backend owns HttpOnly sessions and authorization; no client auth tokens.
  Public profiles/search expose application UUID, nickname/avatar, never email or
  provider identity. Nickname discovery excludes the caller; no email search.
- Admin is an environment email allowlist, not a database role. Backend-rendered
  admin manages brands/models and minimal statistics/users; ordinary users cannot.
- Active brands/models are selectable. A model's brand is immutable on edit.
  Names are trimmed, case-insensitively unique (models within brand); Custom is a
  client option, never a catalog row. NULL chassis means custom/unlisted/unspecified.
  Existing setups retain historical display for inactive catalog entries.
- Owners CRUD setups: title, optional chassis, visibility, structured schema-v1
  suspension/shocks/electronics/rear differential, optional notes and timestamps.
  Only owners write.
  Optional numbers preserve unknown versus explicit zero; supplied data replaces
  the whole technical document. Unsupported versions must not be silently reinterpreted.

## Visibility and friendship

| Viewer | Public | Friends | Private |
| --- | --- | --- | --- |
| Guest | Approved only | Deny | Deny |
| Owner | Read | Read | Read |
| Accepted friend | Read | Read | Deny |
| Other / pending friend | Approved only | Deny | Deny |

Moderation is separate from visibility: new setups at every visibility start
`pending`; only public pending rows enter the admin review queue. Existing public
setups are migrated as `approved`. Public pending/rejected setups remain readable
by owners and accepted friends, including detail, comparison and protected media.
Admin allowlist sessions have explicit `/admin/moderation` preview/photo routes;
being an admin never bypasses visibility on normal API URLs. Public discovery,
search and public profile summaries include only approved public setups.

Approve/Reject consume the exact preview update timestamp, record reviewer/time,
and reject stale or concurrent decisions. Photo insert/delete/replacement, cover
change and crop/frame change atomically reset status to pending, clear reviewer/time
and advance the setup update timestamp. This also applies while friends/private,
so hiding, editing photos and republishing cannot reuse approval. Failed mutations,
identical upload retries, same-cover requests and unchanged crop retries preserve
approval. Text, chassis, notes and technical-data edits preserve moderation state
but invalidate stale review forms. Approval survives visibility-only toggles when
photos are unchanged. A rejected setup stays rejected on text edits; photo changes
resubmit it as pending. Moving a rejected setup away from public and back also
resubmits as pending, never approved. No rejection messages or reporting workflow.
The first 100 pending public rows are reviewed oldest-update first; deciding them
reveals subsequent rows.

Discovery returns approved public setups only, even for owners/friends. Owner lists show all;
profile lists return only backend-authorized items. Inaccessible and missing detail
are indistinguishable. Reads do not grant modification rights. Explore is public;
safe owner profiles show nickname/avatar and public setups without hidden counts.
Only public setup pages are index-eligible; login is prompted for protected actions.

Comparison selects and reads exactly two distinct setups from the same visibility
matrix across all owners. Selection is paginated and rechecks current access; both
comparison sides are authorized together or neither is returned. Different/historical
and custom chassis are allowed. Reads preserve all optional technical data and notes,
including explicit zero versus unknown, without saving a comparison or granting clone
or mutation permissions. Explore remains public-only.

There is one pending/accepted relationship per unordered user pair; no self-friendship.
Only the pending addressee accepts. Requester cancels, addressee rejects, either
accepted participant removes. Pending grants nothing; removal revokes friends access.

## Optional arm spacing

Each axle/arm independently has optional top/lower totals ahead of/behind the arm
in vehicle direction, in mm, symmetric left/right. Finite nonnegative numbers retain
zero/precision; either side can be unknown. Empty blocks/parents are omitted. No
chassis defaults, asymmetry, shim calculator or schema-v1 backfill. Removal uses full
technical-data replacement; unrelated edits preserve spacing and other measurements.

## Optional photos and framing

- Zero to ten photos separate from technical JSONB. Owner uploads/cover/delete/crop
  only. Stable creation-time/UUID order, no captions/manual sorting. First upload is
  cover; deleting cover selects oldest remaining; empty gallery has no cover.
- Save the setup once before upload; failed uploads retain it and successful photos.
  Explicit retries reuse a UUID request_id and identical bytes. Technical edits do
  not change photos; image mutations persist independently.
- Gallery/full/thumbnail reads require current setup visibility; guests see public only.
  Visibility/friendship changes revoke later reads of known URLs. Private no-store
  media is not a public static asset; already downloaded copies cannot be revoked.
- Content-based JPEG/JPG, PNG, WebP and HEVC HEIC/HEIF still images only: at most
  25 MiB and 50 megapixels. GIF/AVIF/BMP/TIFF/SVG/video are unsupported. First frame,
  normalized orientation, stripped metadata, white transparency, JPEG full <=2400px
  and thumbnail <=640px; no upscaling. Only processed images are stored privately.
- Full images retain natural aspect and bytes across thumbnail edits. New thumbnails
  are centered squares. Owners adjust/reset an integer square in normalized full
  pixels; revision conflicts prevent stale overwrite. Failed/cancelled edits preserve
  the saved frame. Legacy photos need no backfill/re-upload; old crops may be NULL.
- Gallery and joined list/detail cover projections carry crop/revision. No per-item
  metadata queries. A frontend placeholder is not a stored photo or count entry.
- Rejections distinguish format/decoding, bytes, pixels and count. Resource termination
  and timeouts are server failures; safe diagnostics never expose native stderr/EXIF.

## Scope boundaries

Do not add by assumption: other disciplines, comments/likes/ratings/favorites,
chat/notifications/teams, setup history, arbitrary attachments, user-created chassis
catalog entries, normalized electronics/shock catalogs, offline synchronization or
native-mobile-specific backend behavior. New scope needs explicit requirements and
any changed [accepted decision](decisions.md), not an old roadmap phase.
