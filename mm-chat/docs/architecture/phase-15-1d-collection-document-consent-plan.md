# Phase 15.1D Collection, Document, and Consent Plan

## Objective and Boundary

Implement the authoritative Knowledge control-plane vertical slice in
Go/Postgres: Personal and Team Collections, logical Documents and immutable
Versions, locked File binding/deletion, Processor Governance, Processing
Consent, durable Processing Jobs, and transactional Knowledge Outbox events.

The existing Next.js/React UI and browser-local Knowledge store remain
unchanged in 15.1D. Python parsing, vector indexing, search, citation assembly,
Team deletion, and automatic frontend server-mode switching remain outside this
slice. Postgres is authoritative; Redis, object storage, Python, and a future
vector database cannot grant access or override a revision fence.

## Locked Product and Authority Decisions

- A Personal Collection belongs only to its Session User. Team Admin authority
  never exposes another user's Personal Collection, Document, Consent, File, or
  existence metadata.
- Active Team Members may list/read Team Collections and Document metadata or
  content. Only active Team Admins may create/update/delete Team Collections,
  bind/replace/reprocess Documents, or change Collection Consent.
- Collection `scope`, Personal owner, and Team binding are immutable. Moving
  content creates a new Collection/Document and tombstones the old binding.
- A logical Document ID is stable. Every source replacement creates an
  immutable Version bound to an existing caller-owned File. The current Active
  Version stays serving until a later verified publish transaction switches it.
- Deleting a Document tombstones its Versions and derived/index state but never
  deletes its Source Files. The File owner deletes an unbound File separately.
- Collection Consent is authoritative for `parse` and `passage_embedding` and
  for Collection-data use in `rerank`/`answer`; User Query Consent is
  authoritative for `query_embedding` and User-query use in `rerank`/`answer`.
  The shared purposes require both applicable Consents and active approved
  Governance bindings.
- Public callers identify a configured Processor alias only. Endpoint, Profile,
  model/API version, Governance revision, and generation are server-selected.

## Public API and DTO Contract

Every route requires a Phase 15.1B Bearer Session. Bodies use strict JSON,
reject unknown fields, and forbid actor/owner/ACL/revision/Governance hints.
Lists use the existing authenticated Team cursor key ring, bound to endpoint,
User, normalized filters, and sort tuple; default `limit=50`, range `1..100`.

```text
POST   /v1/knowledge/collections
GET    /v1/knowledge/collections?scope&teamId&cursor&limit
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
```

```ts
type CollectionScope = "personal" | "team";

interface CreateCollectionRequest {
  name: string;
  description?: string;
  icon?: string;
  color?: string;
  scope: CollectionScope;
  teamId?: string; // required only for team scope
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
  purposes: string[];
  dataTypes: string[];
  policyVersion: string;
  expiresAt?: string;
}
```

`PATCH Collection` accepts only `name`, `description`, `icon`, and `color`.
Names are 1-100 runes/256 bytes; descriptions are at most 2,000 runes/8 KiB;
icon/color values use the existing frontend allowlists. Idempotency keys are
required, 1-128 bytes, and scoped to the actor plus operation aggregate.
Repeated identical Consent PUT/DELETE requests return the current decision and
do not advance a revision twice.

Document lists order by `(created_at DESC, id DESC)` and return the existing
`ApiPage<T>` envelope. Consent reads expose Processor alias, purposes, data
types, policy version, decision, expiry, and decision timestamp; they never
expose endpoint credentials or internal policy manifests.

The server DTO intentionally mirrors the current frontend `Collection` and
`KnowledgeFile` display fields, but transport adapters—not UI components—map
RFC 3339 timestamps/statuses to the existing browser-local shape.

## Permission and Disclosure Order

1. Validate Session, bounded path/query/body syntax, and forbidden fields.
2. Resolve the Collection through Personal ownership or active Team Membership.
3. Return `404 COLLECTION_NOT_FOUND` for absent, deleted, cross-user,
   cross-Team, or inactive-Membership resources.
4. For a visible Team Collection, return `403 TEAM_ADMIN_REQUIRED` only when an
   active Member attempts a management operation.
5. Only after Collection authorization resolve nested Document, Version, File,
   Consent, or Processor state.

Collection lists omit unauthorized rows and totals. Document targeting follows
the same order and uses `404 DOCUMENT_NOT_FOUND`. File binding returns the same
`404 FILE_NOT_FOUND` for unknown, deleted, unavailable, wrong-purpose, or
other-user Files. Processor absence/disablement is disclosed only as
`503 KNOWLEDGE_PROCESSOR_UNAVAILABLE` after resource authorization.

| HTTP | Code                              | Contract                                                          |
| ---- | --------------------------------- | ----------------------------------------------------------------- |
| 400  | `INVALID_COLLECTION_PAYLOAD`      | Invalid Collection JSON, fields, scope, cursor, or display data.  |
| 400  | `INVALID_DOCUMENT_PAYLOAD`        | Invalid File ID, idempotency key, or Document operation body.     |
| 400  | `INVALID_CONSENT_PAYLOAD`         | Invalid Processor, purpose, data type, policy, or expiry input.   |
| 400  | `FORBIDDEN_IDENTITY_FIELD`        | Caller supplies identity, ACL, revision, Profile, or fence data.  |
| 401  | `UNAUTHENTICATED`                 | Bearer Session is absent or invalid.                              |
| 403  | `TEAM_ADMIN_REQUIRED`             | Visible Team Member attempts a management operation.              |
| 403  | `PROCESSING_CONSENT_REQUIRED`     | Required active Consent is missing, revoked, or expired.          |
| 404  | `COLLECTION_NOT_FOUND`            | Collection is absent, deleted, or outside current visibility.     |
| 404  | `DOCUMENT_NOT_FOUND`              | Document is absent/deleted or its Collection is not visible.      |
| 404  | `FILE_NOT_FOUND`                  | File is unavailable, wrong-purpose, deleted, or not caller-owned. |
| 409  | `IDEMPOTENCY_CONFLICT`            | Scoped key was reused for a different canonical request.          |
| 409  | `DOCUMENT_PROCESSING`             | A conflicting nonterminal Version/Job already exists.             |
| 409  | `FILE_IN_USE`                     | File deletion would bypass a live Version binding.                |
| 503  | `KNOWLEDGE_PROCESSOR_UNAVAILABLE` | Active approved Governance binding is unavailable.                |

## Migration 006

Never edit committed migrations `004` or `005`. Add reversible migration `006`:

- add bounded `description`, allowlisted `icon`/`color`,
  `created_by_user_id`, and `idempotency_key` to `knowledge_collections`;
- add scoped Collection idempotency uniqueness and active-name/list indexes;
- add `created_by_user_id` and `idempotency_key` to `knowledge_documents`, plus
  uniqueness scoped to Collection/actor;
- add `created_by_user_id` and `idempotency_key` to
  `knowledge_document_versions`, plus scoped uniqueness and at most one
  nonterminal replacement Version per Document;
- add `visibility_epoch` to `knowledge_documents`; add
  `knowledge_processing_jobs` with immutable Document/Version/File, stage
  (`parse|passage_embedding|purge`), operation
  (`initial|replace|reprocess|purge`), exact Processor Head/Profile, status,
  attempt/lease fields, required Collection/Document/Consent revisions,
  idempotency key, sanitized error code, and timestamps;
- add a current-Consent lookup index including scope, subject, Processor,
  decision, expiry, and supersession state;
- add constraints/indexes required for claim/reclaim and Version/Job replay;
- preserve all pre-`006` rows with nullable compatibility columns, then require
  non-null values for new writes in repository logic.

Committed migrations remain immutable. Add reversible migration `007` to
enforce at most one purge Job per Document/Version/Document-visibility fence
for both fresh and already-migrated databases.
Add reversible migration `008` to reject every Governance Profile UPDATE or
DELETE at the database boundary; policy changes always insert a new Profile.

`006 Down` is for isolated pre-release rollback only. It drops Job rows and the
new indexes/columns without touching `004` Knowledge data. Verification runs
`001→006`, `005→006→Down→006`, and catalog residue checks on PostgreSQL 16.

## Transaction and Lock Contract

Writers use one deterministic order:

```text
target User row(s) in UUID order when account state can change
-> Team row when scope=team
-> active Membership recheck
-> Collection row
-> Document row(s) in UUID order
-> File row(s) FOR UPDATE in sorted UUID order
-> Governance Head/Profile
-> current Consent row / User Query Consent state
-> Processing Job
-> Knowledge Outbox
-> COMMIT
```

Personal operations skip Team/Membership locks. Direct File deletion locks its
owned `files` row `FOR UPDATE`, rechecks `available`/not-deleted state, then
rejects with `409 FILE_IN_USE` if any Version remains
`uploaded|processing|failed|active|purging`. Binding takes the same File lock,
requires metadata `purpose = "knowledge"`, copies the immutable SHA-256 hash,
and inserts Document/Version/Job/Outbox before commit. This serializes bind and
delete. Replacement or publication locks all affected Files in sorted UUID
order; no path may lock File first and then Team/Collection/Document.

Object deletion occurs only after the metadata transaction commits. Failure
leaves an inaccessible object for a durable `file.object.delete.requested`
retry/reconciliation event, never an available metadata row pointing to a
missing object. The worker resolves the private object key from Postgres by
File ID; the key is not copied into the event. Knowledge Document deletion
never calls object deletion.

Collection create and Document bind/replace/reprocess claim their required body
`idempotencyKey` inside the authoritative transaction. Same-key/same-payload
replay returns the original result; same-key/different-payload returns
`409 IDEMPOTENCY_CONFLICT`. PATCH and Consent writes are semantic no-ops when
the canonical requested state already matches. Repeated DELETE by the still-
authorized actor returns `204` without another revision/event. Concurrent
retries therefore produce one state transition and one event.

## Lifecycle and Revision Fences

- First bind requires active Collection Consent for the selected initial parse
  Processor, then creates a `processing` Document, Version `uploaded`, durable
  parse Job, and `knowledge.document.version.requested` event atomically. No
  current Version exists. A later verified parse result creates a separately
  fenced passage-embedding Job; one Job never hides multiple Processor
  authorities in an opaque payload.
- Replacement allocates `source_version = max + 1` under the Document lock.
  The old Active Version remains current; only one nonterminal replacement may
  exist. A failed replacement remains auditable and may be retried/reprocessed.
- Reprocess creates a new Job against the same immutable Version and a
  server-selected active Governance Profile. It does not alter source version,
  current Version, active generation, or artifacts in place.
- Document deletion increments its visibility epoch, tombstones all live
  Versions, marks pending Jobs cancelled/purge-required, and emits tombstones.
  Its post-lock mutation timestamp uses PostgreSQL wall-clock time rather than
  transaction-start `now()`, so waiting behind replace/reprocess cannot regress
  Job completion or Version update timestamps.
- Collection metadata-only edits do not change ACL fences. Visibility/delete
  mutations advance `acl_revision` and `visibility_epoch` exactly once.
- Collection Consent changes advance `collection_processing_revision`; User
  Query Consent changes advance `query_consent_revision`. Authorization also
  checks `expires_at` directly, so expiry blocks calls even before maintenance
  writes the revision/outbox event. Cache TTL may never outlive Consent expiry.
- Governance apply/disable is operator-only through `mm-chat-admin`, never a
  public API. It writes an immutable Profile and advances the exact Head
  revision transactionally; active Heads may reference only approved Profiles.

## Consent Evaluation

PUT resolves the configured Processor alias to its exact active Head/Profile,
validates requested purposes/data types against both Governance and scope, then
supersedes the previous current decision and inserts a new immutable granted
decision. DELETE inserts a revoked decision; history is never updated in place
except to set `superseded_at` on the former current row.

Background parse/index requires active Collection Consent for every external
purpose/data type. Query embedding requires the requesting User's active Query
Consent. Rerank/answer requires active Query Consent plus active Collection
Consent for every selected Collection. No partial Collection drop or automatic
Processor fallback is allowed. Local metadata reads and source download require
ACL but no external-processing Consent.

## Outbox Events and Payloads

Every authoritative mutation writes one or more events in the same transaction:

```text
knowledge.collection.created|updated|tombstoned
knowledge.document.version.requested|reprocess.requested
knowledge.document.tombstoned
knowledge.collection.consent.changed
knowledge.user.query-consent.changed
knowledge.governance.head.changed
knowledge.processing.cancelled
file.object.delete.requested
```

Payloads contain event schema version, aggregate IDs, scope subject IDs,
committed revision/epoch, Document/Version/File hash references, Job ID,
operation, and exact Governance binding where relevant. They exclude object
keys/URLs, filenames, raw file content, email, credentials, tokens, Provider
keys, and free-form errors. `event_id` is replay identity. `BIGSERIAL id` is
allocation order only; consumers rescan claimable rows, deduplicate by
`event_id`, and advance a watermark only across a contiguous applied prefix.

## Module and Existing-Code Changes

- Add `internal/knowledge` as a vertical slice: domain DTOs/errors, Service,
  Postgres Repository, strict HTTP Handler, cursor/idempotency helpers, and
  focused unit/Postgres tests.
- Wire protected `/v1/knowledge/*` and `/v1/me/knowledge/*` routes through the
  existing auth middleware and bounded metric labels.
- Refactor `internal/files.Repository.MarkFileDeleted` into the locked
  knowledge-aware transaction. Preserve owner-only File GET/download routes;
  Team readers use the authorized Knowledge content route.
- Add operator Governance commands to `cmd/admin`; never load secrets or policy
  manifests from public request bodies.
- Keep frontend changes out of this slice. Record the DTO adapter mapping for a
  later minimal-wiring phase.

## Verification Matrix

- **Unit/HTTP:** strict body/query/path bounds, UUIDs, names/display allowlists,
  DTO redaction, cursor binding, idempotency replay/conflict, exact errors, and
  protected route registration.
- **ACL:** two Users and two Teams; Personal owner isolation; Team Admin/Member/
  outsider matrix; removed/disabled Membership denial; no Personal inference by
  Team Admin; File routes remain owner-only.
- **PostgreSQL 16:** Personal/Team CRUD, immutable scope, Version sequencing,
  same-Document current Version FK, File hash/purpose/owner binding, Job/Outbox
  atomicity, Consent history/current uniqueness, Governance exact binding, and
  rollback when any Outbox insert fails.
- **Concurrency:** bind versus File delete, two first binds, two replacements,
  delete versus reprocess, Consent PUT/DELETE races, Membership removal versus
  Team mutation, and same-idempotency retries. Assert one winner, no deadlock,
  no deleted bound File, and revisions equal committed events.
- **Deletion/replay:** Document and Collection tombstones, Source File
  preservation, stale Job cancellation, duplicate/out-of-order Outbox replay,
  contiguous watermark behavior, and reconstruction from Postgres.
- **Promotion:** `gofmt`, `go vet`, `go test -race`, `go test ./...`,
  `govulncheck`, Compose config/build, fresh/replay migration drills, secret
  scan, scoped diff gates, and independent xhigh review with `P0/P1/P2 = 0`.

## Execution Checklist

- [x] **15.1D-1 Contract and migration:** synchronize DTO/error/Consent/File
      contracts and add reversible migration `006` with Job/idempotency gaps.
- [x] **15.1D-2 Collection service:** implement Personal/Team CRUD, cursor
      lists, immutable scope, disclosure order, revisions, and Outbox.
- [ ] **15.1D-3 Document and File binding:** implement logical Document/Version
      lifecycle, locked binding/deletion, content authorization, Job creation,
      reprocess, tombstones, and idempotency.
  - [x] Make direct File deletion lock the owned File row, reject live
        Knowledge bindings, and persist object-cleanup Outbox work atomically.
  - [x] Implement first Document/Version bind with Parse Consent/Governance
        admission and atomic Job/Outbox creation.
  - [x] Expose strict authenticated first-bind HTTP admission.
  - [x] Implement ACL-checked Document list/get and Active-only content routes.
  - [x] Implement immutable replacement Version admission with locked Files,
        Parse authority, idempotency, Job, and Outbox.
  - [x] Implement same-Version reprocess admission with target selection,
        Parse authority, idempotent Job, and fenced Outbox.
  - [x] Implement Document tombstone deletion, Job cancellation, per-Version
        purge admission, visibility fences, and deletion Outbox events.
- [ ] **15.1D-4 Governance and Consent:** implement operator Profile/Head
      management, Collection/User decisions, purpose/data-type validation,
      expiry handling, revision fences, and Outbox.
  - [x] Add operator-only Governance manifest apply/disable commands with
        immutable approved Profiles, atomic Head revisions, and Outbox.
  - [ ] Add Collection Consent reads/grant/revoke with Personal-owner and Team
        Admin ACL, expiry, processing revisions, and Outbox.
  - [ ] Add authenticated User Query Consent reads/grant/revoke with query
        consent revisions, expiry, and Outbox.
- [ ] **15.1D-5 HTTP and wiring:** register protected routes, safe DTOs/errors,
      bounded metrics/logging, and later-frontend adapter documentation.
- [ ] **15.1D-6 Verification:** pass unit/race and real PostgreSQL 16 ACL,
      locking, idempotency, Consent, deletion, migration, and replay gates.
- [ ] **15.1D-7 Promotion:** synchronize tracking/deployment/contracts, pass
      quality/security checks, independent review, and explicit-path commit.

## Rollback and Operational Safety

Before frontend wiring, rollback uses the previous API image and migration
`006 Down` only against isolated/pre-release data. After live Knowledge writes,
rollback is forward-fix: do not drop authoritative Documents, Consent history,
Jobs, or Outbox events. Disable new Knowledge writes, keep reads fail-closed,
drain/replay Outbox from Postgres, and restore service with the same Governance
and revision state. Test fixtures use isolated databases and object prefixes.
