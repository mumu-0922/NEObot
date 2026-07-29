# usermemory Design

## Goals

- Make Postgres authoritative for durable Memory in server mode.
- Keep Memory optional and independent from same-conversation continuity.
- Give users inspect/edit/disable/delete control through existing settings UI.
- Retrieve only relevant entries and make Provider/extraction failure harmless
  to the completed chat answer.

## Non-goals

- Replacing original messages or `conversation_context_summaries`.
- Reader promotion, response caching, or browser-local server authority.
- Persisting search results, Knowledge text, credentials, or vague one-off
  context as automatic Memory.

## Architecture

```text
Server governance UI -> typed frontend API -> HTTP Handler -> Service
  -> migration-060 SECURITY DEFINER capabilities -> canonical Memory authority
Legacy Global UI/API -> migration-060 classified legacy wrappers
  -> migration-055 canonical write/delete capabilities
Chat Handler -> Service.SearchRelevant -> guarded Provider system context
Chat finalize transaction -> ID-only outbox/job -> private Go Memory Worker
  -> strict Provider candidate/decision -> migration-056 Review shadow proposal
Current completed user message -> lexical gate -> strict typed action planner
  -> Go target/scope/revision rebinding -> migration-057 action capability
Assistant finalize transaction -> immutable migration-057 L1 Usage links
Activity polling/undo -> user-scoped migration-057 capabilities
Project/policy/scoped Memory/Review/detail -> migration-060 governance functions
Assistant Activity chip -> current-only migration-060 message hydration
Authenticated export -> repeatable-read canonical scan -> canonical JSONL
  -> age scrypt authenticated stream -> encrypted response temporary file
Encrypted import -> authenticate/validate/secret gate -> mapped dry-run
  -> HMAC package/plan/state fence -> atomic migration-061 ADD-only apply
Deletion authority -> encrypted off-host package -> stopped-backend replay
  -> ID/hash hide + plaintext wipe -> full projection rebuild
Canonical Memory transaction -> migration-058 exact/CJK BM25 projection
Current user message -> unchanged v1 Top 5 -> optional migration-058 shadow
  -> ID/revision/rank-only diagnostics; zero prompt injection
Canonical projection -> migration-059 leased BGE-M3 embedding job
Current user message -> exact + BM25 + vector -> RRF(60)
  -> migration-064 local maximum-cosine admission before document egress
  -> optional strict cloud candidate judge || main-model search_memory route
     concurrently with BGE rerank
  -> judge ordinal intersection OR exact Tool-call release in BGE order
  -> request-local score threshold
  -> 600-target/900-hard token budget
  -> ID/revision/rank-only diagnostics
  -> unchanged v1 prompt/Usage; zero hybrid prompt injection
Current stable Global L1 -> migration-063 leased L3 Persona refresh/embedding
Current user message -> Persona exact + BM25 + vector -> RRF(60) -> BGE rerank
  -> one-Persona/300-token bound -> content-free diagnostics
  -> independent shadow or promoted lower-priority prompt block
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
| BGE-M3 vector space is fixed and lease fenced            | Equal dimensions do not authorize model/profile reuse                                   | Jobs pin revision/hash/epoch/scope/projection/provider configuration before completion |
| RRF(60) fuses independent Top-20/30 lanes                | Exact, BM25, and cosine raw scores are not comparable                                    | Deterministic UUID tie-breaks replace hand-tuned linear weights |
| Hybrid remains a PR8 shadow, not a reader                | Benchmark and observation gates must precede prompt authority                            | v1 Top 5, prompt, Usage, and chat success remain byte-authoritative |
| One default-off flag gates all PR8 Provider calls        | API rerank and Worker embedding must not drift operationally                             | Flag false makes zero Memory embedding/rerank calls while projection/jobs remain rebuildable |
| Provider egress uses redacted transient copies           | SQL/source/hash authority must not be weakened to hide a credential leak                  | Raw query/content stays only at its current authority boundary; secret-only query/document/body skips its Provider lane |
| Cloud judge receives ordinals, not authority             | Model output must never become an ownership, scope, revision, or score authority           | The fixed prompt carries only redacted query/candidate bodies and contiguous request-local ordinals; exact JSON is revalidated locally |
| BGE and cloud judge run concurrently                     | Serial hosted stages cannot reliably fit the unchanged two-second shadow cutoff             | Both must finish under the same bounded context; either failure or provenance drift yields `no_memory` |
| Main model routes before seeing candidates               | The owner selected GPT/DeepSeek and rejected another hidden relevance model                    | The route receives only a redacted query plus the exact no-argument `search_memory` Tool |
| Route and BGE work may overlap                           | Serial model decision plus BGE stages would spend the unchanged two-second cutoff               | Candidate bodies stay inside the authorized BGE boundary and every result is discarded unless one exact Tool Call succeeds |
| Missing Tool arguments are not `{}`                     | A nil Go map can otherwise pass a length-only empty check                                        | The adapter requires a non-nil empty object, non-empty call ID, exact name, and exactly one call |
| Owner egress authority is narrower than injection       | Allowing ordinary personal candidates to reach the configured Provider must not weaken answer relevance or secret isolation | Only `irrelevant` exclusion is egress-authorized under the exact v1 policy; false injection and all forbidden reasons remain unchanged gates |
| PR9 governance is not reader promotion                   | Users need control before scoped retrieval is allowed                                      | Project/Conversation Memory is manageable but v1 Global Top 5 remains the only prompt/Usage source |
| Plaintext is hydrated from current authority only        | Revision/evidence history must not resurrect deleted, disabled, archived, or stale content | Detail and Activity return markers after any lifecycle/epoch/scope-generation fence fails |
| Content classification is duplicated at Go and SQL       | A client label or alternate repository caller must not downgrade Sensitive/secret content  | Go fails fast; migration `060` wrappers/capabilities reclassify before canonical mutation |
| Legacy v1 writes use governed wrappers after `060`       | Keeping old runtime EXECUTE would leave a classification bypass                            | Old grants are revoked on up, restored on down, and repository calls must move with migration |
| Activity undo uses `subjectRevision`                     | Current hydrated revision can differ from the action's undo precondition                    | Frontend sends the immutable Activity subject revision and stale undo becomes Review-required |
| Portability plaintext is streaming-only                  | A temporary JSON archive or database staging table becomes a second uncontrolled fact store | Only encrypted temporary HTTP bytes may touch disk; passphrases remain request/stdin memory |
| External IDs are descriptive, never authoritative        | Source user/Project/Conversation/Memory IDs cannot cross the local ownership boundary        | Package-local refs require current-user mapping and imports receive fresh deterministic local IDs |
| Import is dry-run then ADD-only confirm                  | NOOP/conflict/scope decisions must be reviewable and stable across concurrent governance      | Ten-minute HMAC binds package, mappings, plan, and current authority state; REVIEW never overwrites canonical |
| Settings are suggestions only                            | Import must not silently enable Learn, Use, Sensitive, L2, or L3                              | The UI displays them but migration-061 has no settings-apply import capability |
| Restore replays deletion before backend open             | An older valid backup can resurrect already deleted Memory                                   | Authenticated ID/hash replay wipes matching plaintext and rebuilds projections without a Provider call |
| L3 Persona is rebuildable derived data                    | A compact identity summary must not become a second canonical fact store                      | Members pin current stable Global L1; correction edits L1 and rebuilds rather than patching Persona text |
| L3 promotion is independent from L2                       | A failure or rollback in one derived layer must not corrupt another                            | Separate profile, generation, evidence gates, flags, and prompt block preserve L1/L2 fallback |

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
- decrypted portability input is at most 256 MiB, with at most 1,000 Projects,
  50,000 Memories, 200,000 revisions, 64 KiB per JSONL line, and 2,000 Unicode
  code points per Memory content; encrypted multipart upload is capped at
  300 MiB plus 1 MiB framing overhead.
- passphrases are 12–1,024 bytes; import mappings are at most 256 KiB and plan
  tokens at most 4 KiB. Unknown, duplicate, trailing, non-canonical, unordered,
  count/hash-mismatched, or over-limit input fails closed.
- Development Memory routing accepts only model-bound results carrying
  `memory-search-tool-v1` plus SHA-256
  `f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6`;
  zero calls abstains, while use requires one exact `search_memory({})` call.

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
| Old embedding response crosses an authority change | The job and completion capability recheck revision, hash, epoch, scope generation, projection generation, and Provider configuration timestamp |
| Rerank output becomes stale during Provider work | Record reauthorizes every submitted RRF ID/revision against current user/scope/Sensitive/time/epoch/generation authority |
| Hybrid diagnostics leak content or raw scores | Candidate content is transient only; durable observations contain hashes, IDs, ordinals, bounded counts/status/tokens/duration |
| Irrelevant RRF rows reach the hosted reranker | Migration-064 reauthorizes the exact pending surface and gates document egress on a frozen local maximum-cosine threshold without persisting vector/score data |
| Failed/invalid/late rerank output becomes an unscored fallback | Hybrid selection fails closed to no final/injection; only finite `[0,1]` request-local scores above the frozen threshold can reach the token selector |
| Candidate text manipulates the cloud judge | The server prompt labels query/candidates as untrusted data; strict decoding rejects extra/duplicate keys, prose, scores, duplicate/out-of-range ordinals, oversized output, and trailing values |
| Judge model/prompt changes after authorization | Each result must match the exact configured model ID, prompt version/SHA-256, and decoding profile; drift fails closed before selection |
| Route model sees Memory before it decides to search | `HybridMemoryToolRouter` receives only the secret-redacted query; candidates never enter the Tool-planning request |
| Ambiguous Tool output releases Memory | Exact choice/call/ID/name/non-nil-empty-argument validation rejects missing, null, unknown, duplicate, or non-empty calls |
| Route result is replayed under another model/contract | `routeHybridMemory` rechecks exact model ID, contract version, and SHA-256 before any final row is released |
| Owner authorization is mistaken for blanket egress | Policy-aware scoring permits only `irrelevant`; cross-user, out-of-scope, deleted, secret, superseded, Sensitive-disabled, and untrusted-source remain zero-tolerance failures |
| Query or canonical Memory leaks a credential to retrieval Provider | Shared deterministic classification redacts query, rerank documents, and embedding bodies immediately before egress; fully redacted input makes zero corresponding Provider calls |
| Runtime mutates derived/evidence tables | `go_api_runtime` receives only hybrid prepare/admission/record; `memory_worker_runtime` receives only embedding lease capabilities; both lack table CRUD |
| Client downgrades Sensitive content to normal | Go classification and migration-060 SQL classification take the stricter result; Sensitive-off and secret-like writes fail |
| Governance history leaks deleted plaintext | Detail/Activity join current enabled/lifecycle/epoch/scope authority; deleted sources and purged revisions return marker fields only |
| Old Global writer bypasses PR9 policy | Migration `060` revokes its runtime EXECUTE and grants only classification-aware legacy wrappers |
| Cross-user or stale Project/policy/Review mutation | Auth-derived user plus revision/generation/target/replay checks execute inside pinned SECURITY DEFINER functions |
| Export leaks unrelated or non-current authority | One current-user `REPEATABLE READ, READ ONLY` snapshot emits only current non-deleted L1 canonical rows and mapped Project metadata |
| Archive/passphrase persists outside request memory | Plaintext is streamed directly through age; HTTP uses encrypted-only mode-0600 temporary files and removes them on every path |
| Ciphertext is wrong, truncated, or modified | age authentication must complete before a plan or replay transaction can commit |
| Import uses source IDs for IDOR | Portable refs have no authority; Go normalizes mappings and SQL reauthorizes every Project/Conversation against the authenticated user |
| Import overwrites a current fact | Deterministic resolution emits REVIEW; confirm accepts only the unchanged ADD set |
| Token or authority changes after dry-run | HMAC binds user/package/manifest/mappings/plan/state; confirm rebuilds the plan and SQL rechecks authority in the transaction |
| Import creates a second provenance fiction | Imported rows use `source/authority=import`, fresh IDs, and no local message evidence; optional history uses an explicit import actor |
| Restore resurrects deleted plaintext | Offline replay matches ID+content hash, recreates tombstone/manifest evidence, wipes canonical/revision/evidence plaintext, then rebuilds all eligible projections |
| Runtime directly edits portability authority | API/admin receives pinned function execution only; API and Memory Worker have no import/replay table CRUD |
| Conversation L1 widens into a Scene | Migration `062` accepts only Global or one current Project member scope and rejects Conversation members. |
| Scene output becomes authority | Go/SQL recompute member, sensitivity, watermark, generation, and profile fences; Scene remains derived and has no plaintext PATCH. |
| Stale Scene reaches an answer | Candidate lanes and final record independently reauthorize current members, scope, epoch, generation, Sensitive policy, and active lifecycle. |
| Shadow flag disables deletion | Provider-free stale detection and 24-hour purge run before the refresh gate. |
| Persona leaks Project/Conversation context | Migration `063` accepts only current stable Global L1; Go and SQL reject any outside-authority member. |
| Old Persona survives source or Sensitive drift | Eligibility changes advance L3 generation, remove projection, and reauthorize members after every Provider boundary. |
| Persona becomes editable authority | Governance exposes current content/members/evidence but correction writes governed L1 and rebuilds; there is no Persona plaintext PATCH. |

Known limitation: migrations `062` and `063` ship in shadow with all derived
reader rollout flags default-off. No formal 500-case benchmark plus seven-day/
100-turn canary evidence exists, so neither L2 nor L3 can become active
automatically. The schema-v6 Memory Tool route is also Development-only: no
live GPT/DeepSeek result, frozen policy, or product same-model continuation
exists. The v1 Global Top 5 remains the only default prompt and Usage authority.

## Verification

Required coverage includes settings/CRUD, soft delete, duplicate conflict,
user isolation, related/unrelated CJK relevance, disabled zero-read/zero-write,
secret filtering, Provider failure containment, migration down/up, frontend
server authority, Global-only v1 isolation, scoped ownership constraints, and a
real Provider cross-conversation recall/delete proof. Portability additionally
requires age wrong-passphrase/tamper/truncation tests, strict JSON/multipart and
hard-cap tests, secret zero-persistence, cross-user mapping denial, plan/token/
state drift, confirm replay, imported history chains, deletion replay/projection
rebuild, runtime role denial, and a clean PostgreSQL 17
`060 -> 061 -> 060 -> 061` drill.

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
- 2026-07-28: migration-059 fixed BGE-M3 vector projection, leased embedding,
  RRF(60), BGE rerank, and bounded zero-injection hybrid comparison (Memory v2 PR8).
- 2026-07-28: migration-060 Project/Conversation policy, scoped governance,
  Review decisions, current-only detail/Activity hydration, and classified v1
  compatibility wrappers (Memory v2 PR9).
- 2026-07-28: migration-061 authenticated encrypted Export/Import, ADD-only
  state-fenced confirm, off-host deletion replay, and full projection rebuild
  (Memory v2 PR10).
- 2026-07-28: migration-062 same-scope derived L2 Scene lifecycle, hybrid
  retrieval, governance, evidence-gated promotion, and rollback (Memory v2 PR11).
- 2026-07-29: migration-063 stable Global-L1 derived L3 Persona lifecycle,
  hybrid retrieval, governance, evidence-gated promotion, and rollback
  (Memory v2 PR12).
- 2026-07-29: migration-064 read-only pre-rerank admission and split-calibrated
  request-local relevance abstention; v1 remains prompt/Usage authority and
  hybrid promotion stays disabled (Memory v2 relevance hardening).
- 2026-07-29: strict owner-authorized cloud candidate-judge Development profile,
  ordinal-only contract, concurrent BGE intersection, and runtime Memory task-
  model adapter; no production cloud policy or promotion authority installed.
- 2026-07-29: Development-only main-model `search_memory` routing, exact
  model/contract provenance, concurrent BGE release gate, strict empty-argument
  call validation, and no production Tool/frozen policy or promotion authority.
