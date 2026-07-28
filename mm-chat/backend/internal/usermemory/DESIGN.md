# usermemory Design

## Goals

- Make Postgres authoritative for durable Memory in server mode.
- Keep Memory optional and independent from same-conversation continuity.
- Give users inspect/edit/disable/delete control through existing settings UI.
- Retrieve only relevant entries and make Provider/extraction failure harmless
  to the completed chat answer.

## Non-goals

- Replacing original messages or `conversation_context_summaries`.
- Vector search, response caching, or browser-local server authority.
- Persisting search results, Knowledge text, credentials, or vague one-off
  context as automatic Memory.

## Architecture

```text
Settings UI -> typed frontend API -> HTTP Handler -> Service -> Repository
                                                        -> Postgres 035 + 053 + 055
Chat Handler -> Service.SearchRelevant -> guarded Provider system context
Chat finalize transaction -> ID-only outbox/job -> private Go Memory Worker
  -> bounded Provider extractor -> lease-fenced Service.StoreExtracted adapter
```

`usermemory` owns storage and deterministic relevance. `internal/chat` owns
Provider interaction so the package has no dependency back to chat and no
cycle.

## Key decisions

| Decision                                                 | Reason                                                                              | Consequence                                        |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------- | -------------------------------------------------- |
| Settings and rows are separate tables                    | Settings have user lifetime; rows have individual deletion/source state             | Disable does not destroy user data                 |
| Memory and auto-record default off                       | Durable cross-chat retention requires explicit opt-in                               | Same-chat context never depends on Memory          |
| Tombstoned delete plus asynchronous plaintext purge      | Immediate invisibility and no-resurrection must coexist with durable cleanup          | ID/hash authority remains after online plaintext is wiped |
| Go lexical/CJK Top-5 before vectors                      | Deterministic, provider-free baseline for a single-server store of at most 500 rows | Semantic-only paraphrases may miss                 |
| Retrieval happens after Knowledge/Web query construction | Memory must not rewrite private Knowledge or public-search queries                  | Only the answer Provider sees matches              |
| Extraction runs in a leased private worker               | Provider/Redis/worker failure cannot fail the completed answer                       | PostgreSQL polling and retry may delay new Memory  |
| Metadata contains IDs/counts only                        | Diagnostics must not duplicate private text                                         | Full content is available only through Memory CRUD |
| v1 repository is explicitly Global-only                 | PR2 adds scopes before adding a v2 reader/API                                        | Project/Conversation rows remain invisible to v1 CRUD |
| Scope uniqueness uses three partial indexes              | Exact overrides must coexist across Global, Project, and Conversation                | `ON CONFLICT` must repeat the Global index predicate |
| Canonical revision with one prior snapshot               | Avoid a second mutable fact store while preserving edit evidence                      | Controlled purge is the only permitted revision update |
| Manual authority wins exact automatic collisions         | A model must not silently overwrite an explicit user action                           | Semantic conflict/Review remains deferred to PR5 |

## Validation and limits

- allowed types: `fact`, `preference`, `instruction`, `project`, `warning`,
  `decision`, `context` (automatic extraction rejects vague `context`);
- content: 1–2,000 runes; importance: 1–5; tags: at most 12 × 40 runes;
- active rows read: at most 500; retrieval/extraction output: at most five;
- IDs and source IDs must be UUIDs; all SQL writes are parameterized;
- duplicate create upserts the active normalized row; editing into another
  active row returns `MEMORY_CONFLICT`.

## Threat model and controls

| Threat                                   | Control                                                                                      |
| ---------------------------------------- | -------------------------------------------------------------------------------------------- |
| Cross-user read/write                    | User ID comes from authenticated context and is present in every SQL predicate               |
| Cross-user Project/Conversation scope    | Composite `(scope_id, user_id)` foreign keys with `ON DELETE RESTRICT`                      |
| Prompt injection in stored Memory        | JSON encoding plus a server-owned lower-priority/untrusted instruction; current request wins |
| Secret retention by automatic extraction | Prompt prohibition plus content/tag credential-pattern rejection                             |
| Whole-store disclosure                   | Relevance threshold and hard Top-5 cap; no-hit means no block                                |
| Browser/server authority split           | `ChatApp` excludes IndexedDB Memory when server mode is active                               |
| Extraction outage                        | Durable PostgreSQL jobs, bounded timeout/retry, and lease reclaim; answer state is unchanged  |
| Cross-user or stale worker apply         | Migration `054` lease/user/source/generation/profile-fenced functions and no direct table access |
| Deleted Memory resurrected by an old response | Migration `055` rechecks live epoch, source hash, targeted tombstone, and lease at apply |
| Deleted plaintext retained online        | A provider-free purge job clears canonical/revision/evidence plaintext idempotently |

Known limitation: deterministic lexical/CJK matching is intentionally
conservative and may miss semantic paraphrases. Any future embedding lane must
remain user-scoped, thresholded, bounded, optional, and must preserve the
no-whole-store-injection contract.

## Verification

Required coverage includes settings/CRUD, soft delete, duplicate conflict,
user isolation, related/unrelated CJK relevance, disabled zero-read/zero-write,
secret filtering, Provider failure containment, migration down/up, frontend
server authority, Global-only v1 isolation, scoped ownership constraints, and a
real Provider cross-conversation recall/delete proof.

## Change history

- 2026-07-20: initial Postgres/Go durable Memory boundary (G11.13C).
- 2026-07-28: migration-053 Project/scope/settings foundation with a guarded
  rollback and Global-only v1 repository compatibility (Memory v2 PR2).
- 2026-07-28: migration-054 durable capture outbox/jobs and a separate
  lease-fenced Go Memory Worker (Memory v2 PR3).
- 2026-07-28: migration-055 canonical provenance, append-only revisions,
  targeted tombstones, deletion manifests, and online plaintext purge
  (Memory v2 PR4).
