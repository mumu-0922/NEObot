# Phase 15 Knowledge ACL API Contract

## 1. Status and Scope

This is a **future Phase 15 extension contract**, not an implementation report.
The implemented Phase 13 baseline remains the request-scoped auth/session
boundary in [`auth-session-api.md`](./auth-session-api.md) plus Phase 13
ownership enforcement over the existing file API in
[`file-api.md`](./file-api.md). The current backend has no team, membership,
knowledge collection, knowledge document, or processing-consent schema.

Phase 15 adds small-team knowledge sharing without weakening personal-data
isolation. A Jina credential is available for candidate evaluation; exact
embedding and reranking endpoint entitlement remains unverified. Credential
availability does not grant data-processing consent or bypass any ACL check.

## 2. Trust and Identity Boundary

- Every user authenticates with an independent Phase 13 session. Team members
  never share a login, bearer token, or admin session.
- Public registration is disabled by default. The initial operator is
  bootstrapped out of band; subsequent users enter through a single-use,
  expiring team-admin invitation.
- Raw invitation tokens are delivered only to the invited mailbox. The Team
  Admin API returns `inviteId`, masked email, role, status, and expiry but never
  the token. The server stores only the token hash, binds it to one Team, Role,
  and email address, and rejects it after acceptance, revocation, or expiry.
  Acceptance proves mailbox possession, sets that User's Argon2id password
  hash, and issues only that User's Session.
- After Phase 15 cutover, ordinary `POST /v1/auth/login` uses the user's verified
  email and password. The bootstrap token is limited to initial operator
  provisioning or an audited break-glass path and cannot authenticate members.
- Password recovery uses a hashed, single-use, expiring token delivered only to
  the user's verified email. A team admin cannot set, receive, or reset a
  member's credential because that would expose the member's Personal
  Knowledge. Recovery completion and account disable revoke all active sessions.
- Session expiry or logout never destroys the long-lived credential; the user
  can authenticate again. A system operator may disable an account and revoke
  sessions through an audited maintenance path but receives no impersonation or
  Personal Knowledge access.
- `admin` is a team membership role, not a global impersonation capability.
  An admin cannot assume another user's identity or read that user's personal
  knowledge.
- Go resolves the actor exclusively from the authenticated session.
  Caller-controlled body or query fields that claim current identity or ACL,
  such as `userId`, `ownerId`, `actorId`, `sessionId`, `impersonateUserId`,
  `role`, `teamIds`, `aclGroups`, `allowedCollectionIds`, `aclRevision`, or
  `visibilityEpoch`, fail with `400 FORBIDDEN_IDENTITY_FIELD`. Resource IDs in
  server-defined paths remain inputs but never establish caller identity.

## 3. Entities and Invariants

The Phase 15 migration must introduce explicit authoritative records; JSON
metadata is not an ACL source.

### Team and membership

```ts
type TeamRole = "admin" | "member";

interface Team {
  id: EntityId;
  name: string;
  membershipRevision: number;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}

interface TeamMembership {
  teamId: EntityId;
  userId: EntityId;
  role: TeamRole;
  status: "active" | "removed";
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}
```

- A user has at most one active membership per team.
- Every active team has at least one active admin. Removing, demoting, or
  deactivating the last admin must fail atomically with `409 LAST_TEAM_ADMIN`.
- Membership and role changes are serialized against the team row, increment
  `Team.membershipRevision`, and invalidate authorization snapshots and caches.
- Only active team admins may invite users or change team membership roles.

### Collection and document

```ts
type CollectionScope = "personal" | "team";

interface KnowledgeCollection {
  id: EntityId;
  name: string;
  scope: CollectionScope;
  ownerUserId?: EntityId;
  teamId?: EntityId;
  aclRevision: number;
  visibilityEpoch: number;
  collectionProcessingRevision: number;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}

interface KnowledgeDocument {
  id: EntityId;
  collectionId: EntityId;
  currentVersionId?: EntityId;
  status: "active" | "tombstoned" | "deleted";
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}

interface KnowledgeDocumentVersion {
  id: EntityId;
  documentId: EntityId;
  fileId: EntityId;
  sourceVersion: number;
  visibilityEpoch: number;
  status:
    "uploaded" | "processing" | "active" | "tombstoned" | "purging" | "deleted";
  contentHash: string;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}
```

- A collection is exactly one of:
  - personal: `ownerUserId` is set and `teamId` is null;
  - team: `teamId` is set and `ownerUserId` is null.
- Personal collections are private to their owner. Neither team membership nor
  the admin role grants access to another user's personal collection.
- Team collections are readable and queryable by active team members, but only
  active team admins may create, update, bind, reprocess, or delete team
  documents.
- `KnowledgeDocument` is the stable logical identity and belongs to exactly one
  Collection. Each `KnowledgeDocumentVersion` binds one available server File;
  its `documentId`, `fileId`, `sourceVersion`, and `contentHash` identity fields
  are immutable, while `status` and `visibilityEpoch` may change only through
  fenced transactions with Outbox events. `(documentId, sourceVersion)` is
  unique and monotonic.
- Creating a replacement first writes a Processing Version while the old
  `currentVersionId` remains active. After verification, one transaction points
  `currentVersionId` to the new Active Version, tombstones the old Version,
  advances Fences, and writes Outbox events. Failed candidates never replace the
  current Version.
- Search returns only an `active` logical Document plus its exact Active
  `currentVersionId` in the serving Generation.

### Query-consent and processor-governance state

```ts
type ProcessingPurpose =
  "parse" | "passage_embedding" | "query_embedding" | "rerank" | "answer";

interface UserQueryConsentState {
  userId: EntityId;
  queryConsentRevision: number;
  updatedAt: IsoDateTime;
}

interface ProcessorGovernanceProfile {
  id: EntityId;
  processor: string;
  endpointId: string;
  modelApiVersion: string;
  allowedPurposes: ProcessingPurpose[];
  allowedDataTypes: string[];
  region: string;
  retentionPolicy: string;
  deletionContract: string;
  trainingUse: string;
  status: "candidate" | "approved" | "retired";
  governanceRevision: number;
  manifestHash: string;
}

interface ProcessorGovernanceHead {
  processor: string;
  endpointId: string;
  status: "active" | "disabled";
  activeProfileId?: EntityId;
  activeGovernanceRevision?: number;
  headRevision: number;
  updatedAt: IsoDateTime;
}
```

Only the immutable `approved` Profile referenced by the authoritative Active
Head may authorize a real external call. A Disabled Head has no Active Profile
and immediately invalidates every older Approved Revision. Any endpoint,
model/API version, purpose, region, retention, deletion, or training-use change
creates a new Profile/`governanceRevision`, then atomically advances
`headRevision`; it cannot mutate an Approved Profile in place.

### Processing consent

```ts
type ConsentScope = "collection" | "query";
type ConsentDecision = "granted" | "revoked";

interface ProcessingConsent {
  id: EntityId;
  scope: ConsentScope;
  collectionId?: EntityId;
  userId?: EntityId;
  processor: string;
  governanceProfileId: EntityId;
  governanceRevision: number;
  governanceHeadRevision: number;
  purposes: ProcessingPurpose[];
  dataTypes: string[];
  policyVersion: string;
  decision: ConsentDecision;
  grantedByUserId: EntityId;
  decidedAt: IsoDateTime;
  expiresAt?: IsoDateTime;
}
```

- Collection consent authorizes specified document/candidate data types for one
  processor. It is granted or revoked by the personal owner or a team admin.
- Query consent authorizes specified query data types for one processor and may
  be granted or revoked only by that authenticated user. A team admin cannot
  consent for a member's query.
- Collection consent must set only `collectionId`; query consent must set only
  `userId`. Invalid both/neither shapes fail at the database and API boundary.
- Consent is valid only while its pinned Governance Profile/Revision remains the
  Active Head. A new Governance Profile/Head requires re-consent unless an
  explicit, audited policy compatibility rule proves the disclosed terms and
  data use are unchanged and migrates the Consent Revision transactionally.
- Every call first requires the current Active Governance Head and its pinned
  Approved Profile; processor, endpoint, model/API version, purpose, data type,
  region, Profile Revision, and Head Revision must all match the signed request.
  Additional Consent requirements are operation-specific:

| External operation             | Required Consent after Governance                                       |
| ------------------------------ | ----------------------------------------------------------------------- |
| Parse / passage embedding      | Active Collection Consent for that Processor, Purpose, and Data Type    |
| Query embedding                | Active Query Consent for the authenticated User and Query Data Type     |
| Rerank / answer / LLM evidence | Active User Query Consent plus active Consent for every Collection/Data |

Background parse/index work never waits for a nonexistent Query Consent. Query
embedding cannot use Collection Consent as a substitute for the requesting
user's Query Consent. Rerank/answer fails the whole request if any selected
Collection lacks the required Consent; it cannot silently drop that Collection.

- Consent is explicit, versioned, purpose-limited, auditable, and default-deny.
  UI notice, team membership, public-data classification, or availability of a
  Jina credential is not consent.
- Collection Consent expiry/revocation increments
  `collectionProcessingRevision`; Query Consent expiry/revocation increments
  `UserQueryConsentState.queryConsentRevision`. Governance replacement advances
  the Active Head; disablement writes a Disabled Head with a new `headRevision`.
  The relevant revisions block new calls immediately and enter Authorization
  Fingerprints and Cache Keys; the service must not silently switch Processors
  with different egress semantics.

## 4. Permission Matrix

| Action                                    | Personal: owner | Personal: anyone else | Team: admin | Team: member              |
| ----------------------------------------- | --------------- | --------------------- | ----------- | ------------------------- |
| List/read/query a personal collection     | Allow           | Deny as `404`         | N/A         | N/A                       |
| Manage personal collection/documents      | Allow           | Deny as `404`         | N/A         | N/A                       |
| Grant personal collection consent         | Allow           | Deny as `404`         | N/A         | N/A                       |
| List/read/query an active team collection | N/A             | N/A                   | Allow       | Allow                     |
| Manage team collection/documents          | N/A             | N/A                   | Allow       | `403 TEAM_ADMIN_REQUIRED` |
| Grant team collection consent             | N/A             | N/A                   | Allow       | `403 TEAM_ADMIN_REQUIRED` |
| Invite/manage team memberships            | N/A             | N/A                   | Allow       | `403 TEAM_ADMIN_REQUIRED` |
| Grant the caller's query consent          | Allow           | Allow                 | Allow       | Allow                     |

Search requires an explicit, non-empty `collectionIds` list. Go never silently
adds personal or team collections. A missing, deleted, or unauthorized ID fails
the whole request with `404 COLLECTION_NOT_FOUND`; it is never silently removed
from the search.

## 5. Public Endpoint Surface

Only these Auth Endpoints are unauthenticated: invitation acceptance, login,
recovery request, and recovery completion. They use strict IP/account rate
limits, bounded bodies, uniform timing/response shapes where account discovery
is possible. They never echo Invitation/Recovery Tokens. Invalid Login and
Recovery Request do not disclose account existence; successful Login or Invite
Acceptance returns one newly generated Bearer Session Token plus minimal safe
User metadata exactly once. Every other Endpoint requires an independent Phase
13 Bearer Session. Public self-registration is not exposed.

```http
POST   /v1/teams
GET    /v1/teams
POST   /v1/teams/{teamId}/invites
DELETE /v1/teams/{teamId}/invites/{inviteId}
POST   /v1/auth/invites/accept
POST   /v1/auth/login
POST   /v1/auth/recovery/request
POST   /v1/auth/recovery/complete
DELETE /v1/me/sessions
PATCH  /v1/teams/{teamId}/members/{userId}
DELETE /v1/teams/{teamId}/members/{userId}

POST   /v1/knowledge/collections
GET    /v1/knowledge/collections
GET    /v1/knowledge/collections/{collectionId}
PATCH  /v1/knowledge/collections/{collectionId}
DELETE /v1/knowledge/collections/{collectionId}
POST   /v1/knowledge/collections/{collectionId}/documents
GET    /v1/knowledge/documents/{documentId}
GET    /v1/knowledge/documents/{documentId}/content
POST   /v1/knowledge/documents/{documentId}/versions
POST   /v1/knowledge/documents/{documentId}/reprocess
DELETE /v1/knowledge/documents/{documentId}

PUT    /v1/knowledge/collections/{collectionId}/processing-consents/{processor}
DELETE /v1/knowledge/collections/{collectionId}/processing-consents/{processor}
PUT    /v1/me/knowledge/query-consents/{processor}
DELETE /v1/me/knowledge/query-consents/{processor}
POST   /v1/knowledge/search
```

Collection creation accepts `name`, `scope`, and `teamId` only when
`scope = "team"`. Go derives personal ownership from the session and verifies
team-admin membership before creating a team collection.

Invitation acceptance accepts `{ token, password }` over TLS and creates the
long-lived Argon2id credential before issuing a session. Recovery completion
accepts `{ token, newPassword }`; raw invitation/recovery tokens never appear in
URL paths, access logs, or metrics. Phase 15 versions the login DTO from the
bootstrap-token-only Phase 13 baseline to `{ email, password }`. Recovery request
returns the same response for known and unknown email addresses; only the
verified mailbox receives the raw recovery token. Completing recovery and
`DELETE /v1/me/sessions` revoke all existing sessions before issuing or
requiring a new login.

`POST .../versions` accepts a new caller-owned `fileId` and atomically creates
the next immutable Processing Version plus its Job/Outbox row. The old
`currentVersionId` remains Active until the new Version passes verification;
the publish transaction then switches the pointer and tombstones the old row.
`POST .../reprocess` keeps the Source Version and creates a new Parse/Index Job
for the server-selected Active Profile; public callers cannot choose Model,
Endpoint, Generation, or Governance Profile. Bake-off Shadow Jobs use a private
operator path. Reprocess never mutates active artifacts or the Active Generation
in place.

## 6. File Binding and Deletion

Document Version creation binds an already uploaded Phase 13-owned File:

```json
{
  "fileId": "3e29355e-5d16-4d7d-a6d9-3a1aa8f81443"
}
```

- Go requires an available, non-deleted `purpose = "knowledge"` file owned by
  the caller. A team admin may deliberately bind their own file into a team
  collection; a member may not bind team documents.
- Binding records the immutable File hash and Source Version in one Postgres
  transaction before creating index work. Object keys, buckets, and direct
  object-store URLs remain private.
- `knowledge_document_versions.file_id` uses `ON DELETE RESTRICT`. Both Version
  binding and direct File deletion lock the same `files` row with `FOR UPDATE` before
  checking status/bindings; replacement locks multiple file rows in sorted UUID
  order. Binding inserts the Document Version, Job, and Outbox row before
  commit; first creation also inserts the Logical Document. Direct deletion
  returns `409 FILE_IN_USE` while a live/processing Version binding exists.
  This serialization prevents concurrent bind/delete from leaving an active
  Document pointing at a deleted object.
- Team readers use `/v1/knowledge/documents/{documentId}/content`, where Go
  authorizes the document's collection. Team access is never added to the
  owner-only `/v1/files/{fileId}` routes.
- `DELETE /v1/files/{fileId}` performs the locked check above. The caller must
  delete each authorized Document through the Knowledge endpoint first.
- `DELETE /v1/knowledge/documents/{documentId}` logically tombstones the logical
  Document and every live Version, then queues derived-artifact/index cleanup;
  it does not delete Source Files. After no live binding remains, the File Owner may use the
  existing file deletion endpoint.

## 7. Search Request and Authorization Fences

```ts
interface KnowledgeSearchRequest {
  query: string;
  collectionIds: EntityId[];
  mode: "strict_grounded" | "optional_enrichment";
  limit?: number;
}
```

Go, not the browser or Python service, computes the allowed collection set from
the session user, current Postgres memberships, collection ownership, consent,
and authoritative state. It then issues a short-lived, audience-bound internal
request containing an authorization fingerprint and immutable snapshot. That
snapshot includes Team Membership Revisions, every selected Collection's ACL/
Visibility/Processing Revisions, the User Query Consent Revision, and the exact
Processor Governance Head/Profile ID/Revisions. Python returns source references
and scores only; it does not decide end-user access.

Every dense, sparse, lexical, exact, rerank, hydration, and cache path must
apply the same signed fences inside candidate retrieval, before Top-K:

```text
collection_id IN allowed_collection_ids
document_status = active
document_version_id = authorized_current_version_id
source_version = authorized_current_version
collection_acl_revision = authorized_collection_acl_revision
collection_visibility_epoch = authorized_collection_visibility_epoch
document_visibility_epoch = authorized_document_visibility_epoch
index_generation = authorized_generation
projection_revision >= required_projection_revision
```

Search points must carry at least `collection_id`, `document_id`,
`document_version_id`, `scope`, `owner_user_id` or `team_id`, `source_version`,
`collection_acl_revision`, `collection_visibility_epoch`,
`document_visibility_epoch`, `document_status`, `index_generation`,
`projection_revision`, `source_span_id`, `source_span_hash`, and `active`.
Collection Processing Consent does not change local Search visibility and is
not copied into every Point. Query, rerank, evidence, and answer Cache Keys also
include the Authorization Fingerprint, `team_membership_revision`, every
selected `collection_processing_revision`, `user_query_consent_revision`,
`processor_governance_profile_id`, `processor_governance_revision`,
`processor_governance_head_revision`, and every Collection/Document fence above.

Go reauthorizes every returned source span against current Postgres rows before
loading content or assembling evidence. Strict-grounded answers repeat that
check at the commit boundary; any changed fence discards buffered content and
causes one fully reauthorized retry or a fail-closed response. Stale search
points, caches, or worker results can never override Postgres authorization.

### 7.1 Canonical mutation-to-fence contract

| Authoritative mutation                 | Revision/Fence advanced                                   | Required invalidation/effect                                       |
| -------------------------------------- | --------------------------------------------------------- | ------------------------------------------------------------------ |
| Team member add/remove/role/status     | `team.membership_revision`                                | Revoke old auth snapshots; evict query/evidence caches; no reindex |
| Collection ACL/visibility/delete       | `collection_acl_revision` / `collection_visibility_epoch` | Outbox payload update/tombstone; evict Collection caches           |
| Collection Processor Consent change    | `collection_processing_revision`                          | Block affected egress/jobs; evict caches; no Point rewrite         |
| User Query Consent change              | `user_query_consent_revision`                             | Block new query/rerank/answer calls; evict that User's caches      |
| Document Version publish/delete/status | `document_version_id`, `source_version`, visibility epoch | Outbox point replacement/tombstone; invalidate Evidence            |
| Processor Governance replace/disable   | Profile Revision + Governance Head Revision               | Block old Profile calls; evict Processor-dependent caches          |
| Parser/Embedding/Analyzer shape change | new `index_generation`                                    | Build/verify a new Generation; never mutate Active in place        |

If Consent terms require local Processor-derived Artifact deletion, the Consent
mutation also writes cleanup/rebuild Outbox work and disables the affected
Retrieval Lane until a compliant Artifact is Active. It still does not fake a
Collection ACL/Visibility change or require rewriting every Point merely to
enforce egress authorization.

Collection Scope and Personal Owner/Team binding are immutable. Moving content
between Personal and Team creates a new Collection/Document under newly granted
Consent and tombstones the old binding; it never edits ownership in place.

## 8. Error and Disclosure Contract

| HTTP  | Code                              | When                                                                         |
| ----- | --------------------------------- | ---------------------------------------------------------------------------- |
| `400` | `FORBIDDEN_IDENTITY_FIELD`        | A public request supplies identity, role, ACL, or authorization-fence hints. |
| `400` | `INVALID_COLLECTION_SCOPE`        | Collection scope or its owner/team shape is invalid.                         |
| `401` | `UNAUTHENTICATED`                 | Phase 13 session resolution fails.                                           |
| `401` | `INVALID_CREDENTIALS`             | Email/password or recovery completion is invalid without account disclosure. |
| `403` | `TEAM_ADMIN_REQUIRED`             | An active member attempts a visible team-admin operation.                    |
| `403` | `PROCESSING_CONSENT_REQUIRED`     | Required collection or query consent is absent, expired, or revoked.         |
| `404` | `TEAM_NOT_FOUND`                  | Team is missing, deleted, or outside the caller's membership visibility.     |
| `404` | `COLLECTION_NOT_FOUND`            | Collection is missing, deleted, or outside the caller's ACL.                 |
| `404` | `DOCUMENT_NOT_FOUND`              | Document is missing, deleted, or its collection is outside the caller's ACL. |
| `404` | `FILE_NOT_FOUND`                  | Binding file is missing, deleted, unavailable, or not owned by the caller.   |
| `409` | `LAST_TEAM_ADMIN`                 | A mutation would leave an active team without an admin.                      |
| `409` | `FILE_IN_USE`                     | Direct file deletion would bypass a live knowledge-document binding.         |
| `409` | `PROJECTION_NOT_READY`            | The search projection cannot reach the required revision before deadline.    |
| `410` | `INVITE_NOT_ACTIVE`               | Invitation is accepted, revoked, expired, or otherwise unusable.             |
| `503` | `KNOWLEDGE_PROCESSOR_UNAVAILABLE` | Governance Head/Profile is missing, disabled, stale, or not approved.        |

Use `404` to hide the existence of another user's personal resources and teams
outside the caller's membership. Use `403` only when the actor may know the
resource exists but lacks the required team role or processing consent. List
responses must omit unauthorized resources without exposing counts or IDs.

## 9. Outbox, Tombstones, and Revocation

Postgres is authoritative. Creation, replacement, ACL tightening, membership
change, consent revocation, and deletion write a transactional outbox event in
the same transaction as their new revision/fence.

- ACL tightening or deletion is acknowledged only after the transaction has
  advanced the relevant fence, made the row logically invisible when needed,
  and persisted tombstones for dependent search points, artifacts, and caches.
- Workers apply upserts and deletes idempotently, persist the applied outbox
  watermark, and rescan Postgres after restart or Redis loss. Redis is only a
  wake-up/cache layer.
- Physical search, derived-artifact, processor, and object cleanup may be
  asynchronous. New authorization snapshots reject the old revision
  immediately, even while purge is pending.
- Tombstones and outbox history remain until all supported serving/rollback
  generations and backup windows have crossed the deletion watermark. A
  restore with a missing outbox interval stays unready and rebuilds from
  authoritative Postgres plus retained source artifacts.

## 10. Required Tests

- Auth/invite: separate users receive separate sessions; invitation tokens are
  hashed, single-use, expiring, team/role/email-bound, and revocable; public
  registration is unavailable. Admin invite responses never contain the raw
  Token, which is delivered only to the invited mailbox; Admin cannot consume
  it without mailbox possession. Acceptance creates an Argon2id credential;
  members can log in again after logout/expiry. Login/invite/recovery work
  without a Session, are rate-limited, and do not enumerate accounts. Successful
  Login/Invite Acceptance returns a new raw Session Token exactly once without
  echoing the Invite Token. Recovery Tokens are mailbox-delivered, hashed,
  single-use, and revoke all Sessions;
  Team Admin cannot obtain/reset member credentials or use recovery to access
  Personal Knowledge. Account disable rejects login and revokes all Sessions.
- Identity input: every public DTO rejects caller identity, role, ACL, allowed
  collection, and fence hints before repository or Python calls.
- Personal isolation: a team admin cannot list, get, query, download, mutate,
  or infer another user's personal collection or documents; all targeted
  attempts return the same `404` shape as unknown IDs.
- Team permissions: members can read/query team knowledge but receive `403` on
  document, collection, consent, invite, and membership mutations; admins can
  perform those mutations.
- Collection selection: missing/empty `collectionIds` fails validation; Go
  never adds a personal or team collection implicitly, and any unauthorized ID
  fails the whole request.
- Last-admin invariant: concurrent remove/demote/leave operations cannot commit
  a team with zero admins.
- Consent: Parse/Passage Embedding checks Governance + Collection Consent; Query
  Embedding checks Governance + User Query Consent; Rerank/Answer checks all
  three. A Team Admin cannot grant a member's Query Consent, and credential
  availability alone never enables Jina. Collection, User Query, and Governance
  revisions independently block old calls and Cache entries.
- Search fencing: every retrieval branch receives identical Collection/Document
  ACL, Visibility, Version, Generation, and Projection filters. External-call
  admission and Cache Keys additionally verify Collection Processing, User Query
  Consent, and Governance Head/Profile Revisions; their changes evict affected
  caches without requiring unsafe stale-point acceptance.
- Evidence: Go rejects unauthorized, deleted, hash-mismatched, or stale Python
  source references both before prompt assembly and at strict commit.
- File path: only caller-owned available Knowledge files bind; members cannot
  bind Team Documents; concurrent bind/direct-delete serializes on the same File
  row and cannot create a dangling Document. Direct deletion of a bound File
  returns `409`; replacement creates a new immutable Version while the old
  `currentVersionId` remains Active until verified, and concurrent publishes
  cannot skip/reuse `sourceVersion`. Reprocess cannot mutate Active Artifacts;
  Invitation revocation prevents acceptance; Team content reads never relax
  `/v1/files` ownership.
- Outbox/recovery: mutation plus outbox commit atomically, replay is idempotent,
  Redis loss does not lose work, tombstones propagate to every serving
  generation, and missing replay history forces an unready rebuild.
- Error disclosure: cross-user/cross-team resources use indistinguishable
  `404` responses, while visible role and consent failures use `403` without
  leaking private metadata.
