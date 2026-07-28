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
  -> strict Provider candidate/decision -> migration-056 Review shadow proposal
Current completed user message -> lexical gate -> strict typed action planner
  -> Go target/scope/revision rebinding -> migration-057 action capability
Assistant finalize transaction -> immutable migration-057 L1 Usage links
Activity polling/undo -> user-scoped migration-057 capabilities
Canonical Memory transaction -> migration-058 exact/CJK BM25 projection
Current user message -> unchanged v1 Top 5 -> optional migration-058 shadow
  -> ID/revision/rank-only diagnostics; zero prompt injection
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
| Manual authority wins every automatic conflict           | A model must not silently overwrite an explicit user action                           | PR5 records a pending Review target/revision while canonical remains unchanged |
| Worker reuses validation but not the legacy write method | Candidate limits must stay aligned without preserving PR3 auto-apply                  | `NormalizeCandidateForStorage` is pure; `StoreExtracted` has no production caller |
| Direct action planner is proposal-only                   | Model output cannot become user/scope/target authority                                | Go rebinds visible targets and SQL repeats current epoch/generation/revision checks |
| Direct actions run before Recall                         | The same answer must observe a completed correction/forget, not stale Memory           | Planner failure degrades without blocking the main chat Provider                    |
| Usage is immutable answer provenance                     | Retry must not rewrite which revision was injected                                     | Exact replay succeeds; changed order/content or length fails atomically              |
| Activity stores links, not private text                  | Polling must not create a second Memory/candidate fact store                            | Authenticated reads hydrate current visible content or a deleted marker              |
| Undo is revision fenced                                  | A stale UI action must not overwrite later user changes                                 | Created undo deletes; corrected undo appends restore; stale becomes Review-required  |
| Lexical projection maintenance ignores the shadow flag   | Derived plaintext must never drift while observation is disabled                        | Every canonical/lifecycle/generation mutation refreshes or removes projection in the same transaction |
| PR7 shadow is not a reader                               | Recall promotion needs separate benchmark and rollout authority                         | v1 remains the only prompt/Usage source; compare errors are bounded metadata only |
| Exact and CJK BM25 lanes remain independent              | Their behavior must be measurable before PR8 fusion                                     | PostgreSQL records lane ordinals without persisting query, content copies, or raw scores |

## Validation and limits

- allowed types: `fact`, `preference`, `instruction`, `project`, `warning`,
  `decision`, `context` (automatic extraction rejects vague `context`);
- content: 1–2,000 runes; importance: 1–5; tags: at most 12 × 40 runes;
- active rows read: at most 500; retrieval/extraction output: at most five;
- IDs and source IDs must be UUIDs; all SQL writes are parameterized;
- duplicate create upserts the active normalized row; editing into another
  active row returns `MEMORY_CONFLICT`.
- direct action planner output is at most 16 KiB, contains at most five target
  references, uses schema major one, and requires confidence at least `0.80`
  for mutation; Activity pages are 1–100 and Usage lists are at most five.

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
| Candidate model output mutates canonical | Migration `056` revokes old worker apply and permits only atomic shadow/Review proposal functions |
| Assistant/history/tool text triggers direct mutation | Only the current completed `role=user` message passes the lexical gate; planner intent is fixed before Provider input |
| Forged or stale action target | Go binds IDs/revisions from bounded hydration; SQL rechecks user, scope, epoch, generation, lifecycle, and revision |
| Secret enters a second Provider/store | Local classification rejects before planner egress and persists only source hash/result code |
| Usage replay rewrites answer provenance | Per-assistant advisory lock plus exact immutable replay comparison |
| Deleted/archived/stale Memory leaks through governance APIs | Activity/Usage hydration returns no content and a deleted marker unless every current-state fence passes |
| Undo resurrects stale state | Current revision/epoch/scope/source snapshot checks fail to `review_required` without mutation |
| Shadow ranks unauthorized or stale projection | SQL binds user, scope, Sensitive switch, time, epoch, generation, revision, and hash before either lane |
| Shadow diagnostics leak private text or scores | Observation rows contain only hashes, IDs, revisions, lane ordinals, counts, status, and duration |
| Shadow outage changes the answer | `SearchRelevantWithShadow` completes v1 first and converts compare errors to a fixed failure summary |
| Runtime mutates derived/evidence tables | `go_api_runtime` can execute only the compare function; both runtime roles lack table CRUD |

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
- 2026-07-28: migration-056 candidate/Review shadow and canonical auto-apply
  revocation; v1 reader/API remain unchanged (Memory v2 PR5).
- 2026-07-28: migration-057 direct-user typed actions, immutable answer Usage,
  link-only Activity polling, and revision-safe undo (Memory v2 PR6).
- 2026-07-28: migration-058 transactional exact/CJK BM25 projection and
  default-off, zero-injection lexical comparison (Memory v2 PR7).
