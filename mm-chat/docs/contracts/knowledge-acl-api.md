# Phase 15 Knowledge ACL API Contract

## 1. Status and Scope

This is the authoritative Phase 15 contract for Team and Knowledge ACL
surfaces. Migration `004` provides the identity, Team, Collection,
Document/Version, Governance, Consent, and Knowledge Outbox foundation.
Phase 15.1B and 15.1C now expose independent auth/session plus protected
Team/Membership/Invite services; migration `005` adds Team idempotency, durable
encrypted Invite delivery, and Membership deletion fencing. Phase 15.1D adds
the protected Knowledge APIs and reversible migration `006` for Collection
display/replay metadata, independent Document visibility, and durable
stage-specific Processing Jobs. Team deletion, Python RAG execution, search,
citation minting, and frontend integration remain later slices.

Runtime status: Personal/Team Collection create/list/get/update/delete routes,
ACLs, authenticated cursors, deletion revisions, and transactional Collection
Outbox events are implemented. The internal first-bind transaction now locks
the caller-owned Knowledge File, validates current Parse Consent/Governance,
and creates Document/Version/Job/Outbox atomically; its public routes are not
registered yet. Document read/replace/reprocess/delete, Consent, Governance
command, and search routes remain unimplemented.

The current auth/session baseline is Phase 15.1B in
[`auth-session-api.md`](./auth-session-api.md), with Phase 13 ownership
enforcement over the existing file API in [`file-api.md`](./file-api.md) until
Knowledge endpoints ship. A Jina credential is available for candidate
evaluation; exact embedding and reranking endpoint entitlement remains
unverified. Credential availability does not grant data-processing consent or
bypass any ACL check.

## 2. Trust and Identity Boundary

- Every user authenticates with an independent Phase 15.1B Email/Password
  identity and Bearer Session. Team members never share a login, bearer token,
  or admin session.
- The global compatibility field `CurrentUser.role = "user"` never grants Team
  authority. Every Team permission is resolved from the current Postgres
  `team_memberships` row.
- Public registration is disabled by default. The initial operator is created
  out of band by the operator-only, one-time `admin bootstrap-identity` command;
  it refuses to run after any Credential exists and is neither a password-reset
  nor a break-glass path. Subsequent users enter through a single-use, expiring
  team-admin invitation.
- Raw invitation tokens are delivered only to the invited mailbox. The Team
  Admin API returns `inviteId`, `maskedEmail`, `teamRole`, `status`,
  `deliveryStatus`, and `expiresAt`, but never the token or token hash.
  Migration `004` stores only the Invite token hash in `team_invites`; Phase
  15.1C adds one encrypted durable `identity_mail_outbox` row per Invite so the
  mailbox, raw token, and acceptance URL persist only inside authenticated
  ciphertext.
- Existing active accounts may accept invitations to additional Teams by
  proving the invited mailbox Token and their current Password. New accounts use
  the same `{ token, password }` DTO to set their first Password. Existing
  Credentials are never replaced.
- Ordinary `POST /v1/auth/login` uses the user's verified email and password.
  The current baseline has no `AUTH_BOOTSTRAP_TOKEN`; the old Bootstrap Token is
  rejected and cannot authenticate an operator or member.
- Password recovery uses a hashed, single-use, expiring token delivered only to
  the user's verified email. A Team admin cannot set, receive, or reset a
  member's Credential because that would expose the member's Personal
  Knowledge. Recovery completion and account disable revoke all active sessions.
- Session expiry or logout never destroys the long-lived Credential; the user
  can authenticate again. A system operator may disable an account and revoke
  sessions through an audited maintenance path but receives no impersonation or
  Personal Knowledge access.
- `admin` is a Team Membership role, not a global impersonation capability. An
  admin cannot assume another user's identity or read that user's personal
  knowledge.
- Go resolves the actor exclusively from the authenticated Session.
  Caller-controlled body or query fields that claim current identity or ACL,
  such as `userId`, `ownerId`, `actorId`, `sessionId`, `impersonateUserId`,
  `role`, `teamIds`, `aclGroups`, `allowedCollectionIds`, `aclRevision`, or
  `visibilityEpoch`, fail with `400 FORBIDDEN_IDENTITY_FIELD`. Only the
  documented Team write DTOs may carry `teamRole`; bare `role` remains
  forbidden everywhere. Resource IDs in server-defined paths remain inputs but
  never establish caller identity.

## 3. Entities and Invariants

Migration `004` already introduced explicit authoritative records; Phase
15.1C adds reversible `005` columns/indexes and encrypted Invite-delivery
state. JSON metadata is not an ACL source.

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

interface TeamInvite {
  id: EntityId;
  teamId: EntityId;
  invitedByUserId: EntityId;
  email: string;
  role: TeamRole;
  status: "pending" | "accepted" | "revoked";
  expiresAt: IsoDateTime;
  acceptedAt?: IsoDateTime;
  revokedAt?: IsoDateTime;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}

interface IdentityMailOutbox {
  id: EntityId;
  inviteId: EntityId;
  keyId: string;
  deliveryStatus: "pending" | "processing" | "sent" | "failed" | "cancelled";
  attemptCount: number;
  availableAt: IsoDateTime;
  leasedUntil?: IsoDateTime;
  createdAt: IsoDateTime;
  updatedAt: IsoDateTime;
}
```

- `team_memberships` and `team_invites` already exist in migration `004`; Phase
  15.1C migration `005` adds nullable scoped `idempotencyKey` support plus the
  durable encrypted `identity_mail_outbox`.
- A user has at most one active membership per team.
- Every active team has at least one active admin whose User account is active
  and not deleted. Removing, demoting, self-leaving, or disabling the last
  usable admin must fail atomically with `409 LAST_TEAM_ADMIN`.
- Membership add/reactivate/role/remove/self-leave changes are serialized
  against the team row, increment `Team.membershipRevision` exactly once, and
  invalidate authorization snapshots and caches.
- A removed membership may be reactivated only through a new mailbox Invite.
- Only active team admins may rename a Team, issue/list/revoke Invites, change
  another Member's role, or remove another Member. Active Members may list
  visible Team/member-safe metadata and leave their own Membership.
- Public Team write DTOs use `teamRole`; bare `role` remains forbidden. Storage
  rows keep `role` as the authoritative database field.
- Each Invite owns exactly one durable Mail Outbox row. Raw token, mailbox, and
  acceptance URL are never returned to admins and persist only inside
  authenticated ciphertext; safe API responses expose only `deliveryStatus`.

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

| Action                                    | Personal: owner | Personal: anyone else | Team: admin                   | Team: member              |
| ----------------------------------------- | --------------- | --------------------- | ----------------------------- | ------------------------- |
| List/get Team metadata                    | N/A             | N/A                   | Allow                         | Allow                     |
| List Team members                         | N/A             | N/A                   | Allow                         | Allow                     |
| Leave own Team membership                 | N/A             | N/A                   | Allow / `409 LAST_TEAM_ADMIN` | Allow                     |
| Rename Team or manage Invites             | N/A             | N/A                   | Allow                         | `403 TEAM_ADMIN_REQUIRED` |
| Change/remove another Team member         | N/A             | N/A                   | Allow                         | `403 TEAM_ADMIN_REQUIRED` |
| List/read/query a personal collection     | Allow           | Deny as `404`         | N/A                           | N/A                       |
| Manage personal collection/documents      | Allow           | Deny as `404`         | N/A                           | N/A                       |
| Grant personal collection consent         | Allow           | Deny as `404`         | N/A                           | N/A                       |
| List/read/query an active team collection | N/A             | N/A                   | Allow                         | Allow                     |
| Manage team collection/documents          | N/A             | N/A                   | Allow                         | `403 TEAM_ADMIN_REQUIRED` |
| Grant team collection consent             | N/A             | N/A                   | Allow                         | `403 TEAM_ADMIN_REQUIRED` |
| Invite/manage team memberships            | N/A             | N/A                   | Allow                         | `403 TEAM_ADMIN_REQUIRED` |
| Grant the caller's query consent          | Allow           | Allow                 | Allow                         | Allow                     |

Search requires an explicit, non-empty `collectionIds` list. Go never silently
adds personal or team collections. A missing, deleted, or unauthorized ID fails
the whole request with `404 COLLECTION_NOT_FOUND`; it is never silently removed
from the search.

## 5. Public Endpoint Surface

Only Login, Invite Acceptance, Recovery Request, and Recovery Completion are
unauthenticated. `POST /v1/auth/invites/accept` stays public because it proves
mailbox possession rather than Team-admin authority. Every `/v1/teams` route
requires an independent Phase 15.1B Bearer Session. Public self-registration is
not exposed. Team services are Phase 15.1C; the Knowledge routes below remain
later Phase 15 surfaces but inherit the same auth boundary.

```http
POST   /v1/teams
GET    /v1/teams?cursor&limit
GET    /v1/teams/{teamId}
PATCH  /v1/teams/{teamId}
GET    /v1/teams/{teamId}/members?cursor&limit
DELETE /v1/teams/{teamId}/membership
PATCH  /v1/teams/{teamId}/members/{userId}
DELETE /v1/teams/{teamId}/members/{userId}
POST   /v1/teams/{teamId}/invites
GET    /v1/teams/{teamId}/invites?cursor&limit
DELETE /v1/teams/{teamId}/invites/{inviteId}
POST   /v1/auth/invites/accept
POST   /v1/auth/login
POST   /v1/auth/recovery/request
POST   /v1/auth/recovery/complete
POST   /v1/auth/logout
GET    /v1/me
DELETE /v1/me/sessions

POST   /v1/knowledge/collections
GET    /v1/knowledge/collections
GET    /v1/knowledge/collections/{collectionId}
PATCH  /v1/knowledge/collections/{collectionId}
DELETE /v1/knowledge/collections/{collectionId}
POST   /v1/knowledge/collections/{collectionId}/documents
GET    /v1/knowledge/collections/{collectionId}/documents?cursor&limit
GET    /v1/knowledge/documents/{documentId}
GET    /v1/knowledge/documents/{documentId}/content
POST   /v1/knowledge/documents/{documentId}/versions
POST   /v1/knowledge/documents/{documentId}/reprocess
DELETE /v1/knowledge/documents/{documentId}

GET    /v1/knowledge/collections/{collectionId}/processing-consents
PUT    /v1/knowledge/collections/{collectionId}/processing-consents/{processor}
DELETE /v1/knowledge/collections/{collectionId}/processing-consents/{processor}
GET    /v1/me/knowledge/query-consents
PUT    /v1/me/knowledge/query-consents/{processor}
DELETE /v1/me/knowledge/query-consents/{processor}
POST   /v1/knowledge/search
```

### Team service DTOs and paging

```json
POST /v1/teams
{ "name": "Research", "idempotencyKey": "..." }

PATCH /v1/teams/{teamId}
{ "name": "Research Ops" }

PATCH /v1/teams/{teamId}/members/{userId}
{ "teamRole": "admin" }

POST /v1/teams/{teamId}/invites
{
  "email": "user@example.com",
  "teamRole": "member",
  "idempotencyKey": "..."
}
```

```ts
type TeamRole = "admin" | "member";

interface TeamDto {
  id: string;
  name: string;
  membershipRevision: number;
  myMembership: {
    teamRole: TeamRole;
    status: "active";
    joinedAt: string;
    updatedAt: string;
  };
  createdAt: string;
  updatedAt: string;
}

interface TeamMemberDto {
  userId: string;
  displayName: string;
  teamRole: TeamRole;
  status: "active";
  joinedAt: string;
  updatedAt: string;
}

interface TeamInviteDto {
  id: string;
  teamId: string;
  maskedEmail: string;
  teamRole: TeamRole;
  status: "pending" | "accepted" | "revoked" | "expired";
  deliveryStatus: "pending" | "processing" | "sent" | "failed" | "cancelled";
  expiresAt: string;
  createdAt: string;
  updatedAt: string;
}

interface ApiPage<T> {
  items: T[];
  nextCursor?: string;
}
```

- `teamId`, target `userId`, actor, ACL, and revision fields are path/context
  derived and forbidden in bodies; public Team writes accept `teamRole`, never
  bare `role`.
- Team names are trimmed, valid UTF-8, free of control/format characters,
  1-100 runes, and at most 256 bytes.
- `POST /v1/teams` and `POST /v1/teams/{teamId}/invites` require an
  `idempotencyKey` of 1-128 bytes. Duplicate scoped keys return
  `409 IDEMPOTENCY_CONFLICT` and cannot emit a second Invite Token or email.
- Lists use opaque, versioned, HMAC-SHA-256-authenticated cursors with
  `limit=50` by default and a range of `1..100`. Encoded cursors above 1024
  bytes fail before decode. Canonical signed content binds `keyId`, contract
  version, endpoint/`resourceKind`, request User ID, optional Team ID,
  normalized filter digest, and the sort tuple; verification uses constant-time
  comparison. An active signing key plus verify-only key ring supports bounded
  rotation. A cursor cannot be replayed across users, Teams, filters, or list
  endpoints and is pagination state, never an authorization credential. Every
  page rechecks current Membership and returns no total count. Team and Invite
  sort order is `(created_at DESC, id DESC)`; Member sort order is
  `(created_at ASC, user_id ASC)`.
- Invite responses expose only the masked mailbox plus `teamRole`, `status`,
  `deliveryStatus`, and expiry metadata. `POST /v1/teams/{teamId}/invites`
  fails closed with `503 INVITE_DELIVERY_UNAVAILABLE` when SMTP, encryption, or
  durable sender admission is unavailable.
- `DELETE /v1/teams/{teamId}/membership` is self-leave only; removing another
  member uses the admin-only `/members/{userId}` route.
- `POST /v1/auth/invites/accept` keeps the same `{ token, password }` DTO for
  both new and existing accounts. Acceptance succeeds only after the durable
  Invite delivery row is `sent`; wrong passwords, stale Credential revisions,
  unsent mail, and branch races all collapse to `410 INVITE_NOT_ACTIVE`.

### Knowledge service DTOs, paging, and idempotency

```ts
type CollectionScope = "personal" | "team";

interface CreateCollectionRequest {
  name: string;
  description?: string;
  icon?: string;
  color?: string;
  scope: CollectionScope;
  teamId?: string;
  idempotencyKey: string;
}

interface CollectionDto {
  id: string;
  name: string;
  description: string;
  icon: string;
  color: string;
  scope: CollectionScope;
  teamId?: string;
  permissions: { read: true; manage: boolean; manageConsent: boolean };
  aclRevision: number;
  visibilityEpoch: number;
  collectionProcessingRevision: number;
  createdAt: string;
  updatedAt: string;
}

interface BindDocumentRequest {
  fileId: string;
  idempotencyKey: string;
}

interface ReprocessDocumentRequest {
  idempotencyKey: string;
}

interface KnowledgeDocumentDto {
  id: string;
  collectionId: string;
  status: "processing" | "active" | "tombstoned";
  currentVersion?: KnowledgeDocumentVersionDto;
  pendingVersion?: KnowledgeDocumentVersionDto;
  createdAt: string;
  updatedAt: string;
}

interface KnowledgeDocumentVersionDto {
  id: string;
  sourceVersion: number;
  file: { id: string; name: string; mimeType: string; byteSize: number };
  status: "uploaded" | "processing" | "failed" | "active" | "tombstoned";
  createdAt: string;
  updatedAt: string;
  errorCode?: string;
}

interface PutConsentRequest {
  purposes: (
    "parse" | "passage_embedding" | "query_embedding" | "rerank" | "answer"
  )[];
  dataTypes: string[];
  policyVersion: string;
  expiresAt?: string;
}

interface ProcessingConsentDto {
  processor: string;
  purposes: string[];
  dataTypes: string[];
  policyVersion: string;
  decision: "granted" | "revoked";
  expiresAt?: string;
  decidedAt: string;
}
```

Collection creation accepts `teamId` only when `scope = "team"`; it is required
for Team scope and forbidden for Personal scope. Go derives Personal ownership
from the Session and verifies active Team-admin Membership before Team creation.
Collection scope/owner/Team are immutable. PATCH accepts only `name`,
`description`, `icon`, and `color`; names are 1-100 runes/256 bytes,
descriptions at most 2,000 runes/8 KiB, and icon/color use the current frontend
allowlists.

Collection and Document lists use the authenticated cursor contract above;
Collection/Document order is `(created_at DESC, id DESC)`. Consent reads expose
only Processor alias, purposes, data types, policy version, decision, expiry,
and decision time—not endpoint credentials or internal manifests.
Collection Consent accepts only `parse|passage_embedding|rerank|answer`; User
Query Consent accepts only `query_embedding|rerank|answer`. An optional
`expiresAt` must be a future RFC 3339 timestamp within the configured policy
horizon.

Collection create and Document bind/version/reprocess require a 1-128 byte body
`idempotencyKey`. Postgres persists a canonical request hash. Same-key/same-
payload replay returns the original result; same-key/different-payload returns
`409 IDEMPOTENCY_CONFLICT`. PATCH and Consent writes are semantic no-ops when
the requested state already matches. Repeated DELETE by the still-authorized
actor returns `204` without another revision or Outbox event.

Recovery completion accepts `{ token, newPassword }`; raw invitation/recovery
Tokens never appear in URL paths, queries, access logs, or metrics. Invite email
links use a client-side `#token=...` fragment, which the future frontend clears
before posting the Token in the acceptance JSON body. Phase 15 versions the
Login DTO from the bootstrap-token-only Phase 13 baseline to `{ email,
password }`. Recovery Request returns the same response for known and unknown
email addresses; only the verified mailbox receives the raw Recovery Token.
Completing recovery and `DELETE /v1/me/sessions` revoke all existing Sessions
before issuing or requiring a new Login.

`POST .../versions` accepts a new caller-owned `fileId` and atomically creates
the next immutable Processing Version plus its stage-specific Job/Outbox row.
The first bind and replacement require active Collection Consent for the
server-selected Parse Processor. The old
`currentVersionId` remains Active until the new Version passes verification;
the publish transaction then switches the pointer and tombstones the old row.
`POST .../reprocess` keeps the Source Version and creates a new Parse Job for
the server-selected Active Profile; a verified Parse result later creates a
separately fenced Passage Embedding Job. Public callers cannot choose Model,
Endpoint, Generation, Governance Profile, or Job stage. Bake-off Shadow Jobs use
a private operator path. Reprocess never mutates active artifacts or the Active
Generation in place.

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
  binding and direct File deletion lock the same `files` row with `FOR UPDATE`
  before checking status/bindings; replacement locks multiple file rows in
  sorted UUID order. Binding inserts the Document Version, Job, and Outbox row before
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
  it does not delete Source Files. After no live binding remains, the File Owner
  may use the existing file deletion endpoint. Metadata deletion commits before
  object deletion and emits `file.object.delete.requested`; failed physical
  deletion is retried by File ID without exposing the private object key in the
  event.

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

| HTTP  | Code                              | When                                                                                       |
| ----- | --------------------------------- | ------------------------------------------------------------------------------------------ |
| `400` | `FORBIDDEN_IDENTITY_FIELD`        | A request supplies actor, target identity, bare `role`, ACL, or authorization-fence hints. |
| `400` | `INVALID_TEAM_PAYLOAD`            | Team name, UUID, cursor, limit, JSON shape, or Team body fields are invalid.               |
| `400` | `INVALID_INVITE_PAYLOAD`          | Invite mailbox, `teamRole`, or `idempotencyKey` is invalid.                                |
| `400` | `INVALID_MEMBERSHIP_PAYLOAD`      | Membership body shape or `teamRole` is invalid.                                            |
| `400` | `INVALID_COLLECTION_SCOPE`        | Collection scope or its owner/team shape is invalid.                                       |
| `400` | `INVALID_COLLECTION_PAYLOAD`      | Collection JSON, display fields, UUID, cursor, limit, or body shape is invalid.            |
| `400` | `INVALID_DOCUMENT_PAYLOAD`        | File ID, idempotency key, or Document operation body is invalid.                           |
| `400` | `INVALID_CONSENT_PAYLOAD`         | Processor, purpose, data type, policy version, or Consent body is invalid.                 |
| `401` | `UNAUTHENTICATED`                 | Phase 15.1B Bearer Session resolution fails.                                               |
| `401` | `INVALID_CREDENTIALS`             | Email/password or recovery completion is invalid without account disclosure.               |
| `403` | `TEAM_ADMIN_REQUIRED`             | An active Member attempts a visible Team-admin operation.                                  |
| `403` | `PROCESSING_CONSENT_REQUIRED`     | Required collection or query consent is absent, expired, or revoked.                       |
| `404` | `TEAM_NOT_FOUND`                  | Team is missing, deleted, or outside the caller's active Membership visibility.            |
| `404` | `TEAM_MEMBER_NOT_FOUND`           | A visible admin targets an absent, removed, or cross-Team Member.                          |
| `404` | `INVITE_NOT_FOUND`                | A visible admin targets an absent or cross-Team Invite.                                    |
| `404` | `COLLECTION_NOT_FOUND`            | Collection is missing, deleted, or outside the caller's ACL.                               |
| `404` | `DOCUMENT_NOT_FOUND`              | Document is missing, deleted, or its collection is outside the caller's ACL.               |
| `404` | `FILE_NOT_FOUND`                  | Binding file is missing, deleted, unavailable, or not owned by the caller.                 |
| `409` | `LAST_TEAM_ADMIN`                 | A mutation or account disable would leave a Team without another usable admin.             |
| `409` | `INVITE_CONFLICT`                 | An active Membership or pending Invite already exists for the Team/mailbox.                |
| `409` | `IDEMPOTENCY_CONFLICT`            | A scoped write key is reused for a different canonical request.                            |
| `409` | `DOCUMENT_PROCESSING`             | A conflicting nonterminal Document Version or Processing Job already exists.               |
| `409` | `FILE_IN_USE`                     | Direct file deletion would bypass a live knowledge-document binding.                       |
| `409` | `PROJECTION_NOT_READY`            | The search projection cannot reach the required revision before deadline.                  |
| `410` | `INVITE_NOT_ACTIVE`               | Invitation is accepted, revoked, expired, unsent, or otherwise unusable.                   |
| `503` | `INVITE_DELIVERY_UNAVAILABLE`     | SMTP, encryption, or durable sender admission is unavailable.                              |
| `503` | `KNOWLEDGE_PROCESSOR_UNAVAILABLE` | Governance Head/Profile is missing, disabled, stale, or not approved.                      |

Authorization order on Team routes is fixed: validate Session and bounded
input, resolve Team visibility, distinguish visible Member from Admin, then
resolve nested Member/Invite IDs. Use `404` to hide the existence of another
user's personal resources and Teams outside the caller's Membership. Use `403`
only when the actor may know the resource exists but lacks the required Team
role or processing consent. Repeated deletion of an already revoked Invite is
`204`; accepted or expired Invites return `410`. List responses must omit
unauthorized resources without exposing counts or IDs.

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

- Team services: Team create/list/get/rename/member/invite routes use Phase
  15.1B Bearer Sessions, strict JSON DTOs, `teamRole` writes, no bare `role`,
  default `limit=50`, `1..100` bounds, authenticated HMAC cursors, and the
  fixed `404 -> 403` visibility/order contract.
- Auth/invite: separate users receive separate Sessions; Invite Tokens are
  hashed, single-use, expiring, Team/teamRole/email-bound, and revocable;
  public registration is unavailable. Admin invite responses expose only masked
  email, `teamRole`, `status`, `deliveryStatus`, and expiry. New-account
  acceptance creates an Argon2id Credential; existing-account acceptance reuses
  the Credential, requires the current Password, and fails closed on stale
  Credential fences or unsent mail. Login/invite/recovery work without a
  Session, are rate-limited, and do not enumerate accounts. Successful
  Login/Invite Acceptance returns a new raw Session Token exactly once without
  echoing the Invite Token. Recovery Tokens are mailbox-delivered, hashed,
  single-use, and revoke all Sessions; Team admins cannot obtain/reset member
  credentials, use recovery to access Personal Knowledge, or disable the last
  usable Team admin.
- Identity input: every public DTO rejects caller identity, bare `role`, ACL,
  allowed collection, and fence hints before repository or Python calls; only
  documented Team write DTOs may use `teamRole`.
- Personal isolation: a Team admin cannot list, get, query, download, mutate,
  or infer another user's personal collection or documents; all targeted
  attempts return the same `404` shape as unknown IDs.
- Team permissions: Members can read/query Team Knowledge, list safe
  Team/member metadata, and self-leave; admins can rename Teams, manage
  Invites, and change/remove other Memberships.
- Collection selection: missing/empty `collectionIds` fails validation; Go
  never adds a personal or Team collection implicitly, and any unauthorized ID
  fails the whole request.
- Team invariants: scoped idempotency uniqueness holds for Team create and
  Invite create, only one pending Invite may exist per Team/mailbox, removed
  Members rejoin only through a new mailbox Invite, and concurrent
  remove/demote/leave/disable operations cannot commit a Team with zero usable
  admins.
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
- File path: only caller-owned available Knowledge files bind; Members cannot
  bind Team Documents; concurrent bind/direct-delete serializes on the same File
  row and cannot create a dangling Document. Direct deletion of a bound File
  returns `409`; replacement creates a new immutable Version while the old
  `currentVersionId` remains Active until verified, and concurrent publishes
  cannot skip/reuse `sourceVersion`. Reprocess cannot mutate Active Artifacts;
  Invite revocation prevents acceptance; Team content reads never relax
  `/v1/files` ownership.
- Outbox/recovery: Team Membership/Invite/account-disable mutation plus outbox
  commit atomically; encrypted `identity_mail_outbox` rows provide durable
  at-least-once delivery with a stable Message-ID, acceptance succeeds only
  after `sent`, revoke/worker/accept races never permit a revoked or unsent
  Token, replay is idempotent, Redis loss does not lose work, tombstones
  propagate to every serving generation, and missing replay history forces an
  unready rebuild.
- Error disclosure: cross-user/cross-Team resources use indistinguishable
  `404` responses, while visible role and consent failures use `403` without
  leaking private metadata.
