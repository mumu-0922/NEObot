# Memory v2 schema foundation

Memory v2 keeps Go and PostgreSQL authoritative. Migrations `053` through `070`
add the Project/scope foundation, durable worker, canonical provenance/delete,
candidate/Review and safe-add authority, direct-user action/Activity/Usage,
rebuildable lexical/vector projections, fixed product Tool hydration, and
content-free worker/user health. Public legacy Memory CRUD remains Global-only;
the active product reader is the separately gated fixed BGE/Luna Memory Tool
and has no v1 fallback.

## Authority model

```text
authenticated user
  -> Global Memory
  -> Project -> Project Memory
  -> Conversation -> Conversation Memory
```

`projects` is a first-class user-owned entity. Conversation membership and
Memory scope use composite `(resource_id, user_id)` foreign keys so an ID from
another user cannot become authority. Scope foreign keys use `ON DELETE
RESTRICT`; future Project/Conversation deletion must execute its reviewed
impact, tombstone, and purge flow before deleting the parent.

`source_conversation_id` remains provenance only. It does not make a Global
Memory Conversation-scoped.

## Additive defaults and backfill

- Existing settings are unchanged. Sensitive Memory defaults off; L2 and L3
  modes default to `inherit`.
- Existing Memories become Global, with null scope foreign keys and scope
  generation one.
- Existing Conversations have no Project, generation one, and independent
  Use/Learn modes set to `inherit`.
- The three active-content unique indexes admit the same normalized content in
  different scopes but reject duplicates inside one exact scope.

## Runtime boundary

PR2 keeps `/v1/memory-settings` and `/v1/memories` on the v1 Global view. The
Go repository explicitly creates and filters `scope_type='global'`; it cannot
list, edit, delete, or mark a Project/Conversation Memory as used. The new
`projects` table receives no `go_api_runtime` grant in PR2.

PR3–PR5 add an ID-only PostgreSQL outbox/private worker, revision/evidence/
tombstone authority, and proposal-only candidate Review shadow. Automatic
Provider output cannot mutate canonical Memory.

PR6 adds three backend seams without changing the reader or frontend:

```text
current completed user message
  -> lexical gate + strict action proposal
  -> Go and PostgreSQL authority rebinding
  -> direct remember/correct/forget or Review-required result

assistant finalize -> immutable L1 Usage links
action/review/dead-letter -> link-only Activity -> safe undo capability
```

The Memory task model is preferred when configured; only an unconfigured
Memory task model falls back to the current chat Provider/model. Secret input
is rejected before this planner call. Activity and Usage rows copy no Memory,
query, prompt, embedding, or raw score. Authenticated reads hydrate only a
current visible Memory, otherwise they return a deleted marker.

An exact bounded referential remember follow-up such as “那你写进去呀” or
standalone “记住” uses the nearest preceding completed user row as candidate
facts. Standalone `记住它/这个/这条`, `记下来`, and `记一下` use the
same lane; generic standalone `保存` and `写进去` do not. Intervening
assistant and incomplete rows are never factual authority. The current message
remains the sole action authority, SQL source, and assistant parent; the prior
user text enters only a separately named v2 planner field after local Secret
classification and deterministic redaction. Missing, Secret, or fully redacted
references make zero planner calls/mutations, while a current message that
already contains the complete fact stays on the unchanged v1 input and hash.
The planner no longer relies on free-form JSON. It must issue exactly one named
required `propose_memory_action_v1` Tool Call with strict semantic arguments,
thinking disabled, temperature zero, and bounded output. The versioned Tool
name—not a model-supplied field—binds the canonical output schema version;
Go then repeats exact-key and semantic validation. Plain text, zero/multiple or
wrong calls, and malformed arguments fail as `PLANNER_OUTPUT_INVALID`, while
Provider/transport failure is classified separately as
`PLANNER_PROVIDER_FAILED`. Both paths leave canonical Memory unchanged and let
the normal answer continue.
The server then maps only the bounded direct-action status/action into an
authoritative answer System instruction. Applied/NOOP results are confirmed;
rejected/review/failed results are not presented as success. The instruction
contains no Memory content, ID, revision, hash, or raw result code and prevents
the answer model from falsely claiming that it lacks Memory permission.

PR7 and PR8 add retrieval evidence without promoting a reader:

```text
canonical Memory transaction
  -> migration-058 exact/CJK BM25 projection
  -> migration-059 fixed BGE-M3 vector status + leased embedding job

current completed user query
  -> unchanged v1 Top 5 (prompt + Usage authority)
  -> raw query to SQL source/hash + lexical authority
  -> transient secret-redacted query/body to BGE Provider boundaries
  -> exact Top 20 + BM25 Top 30 + vector Top 30
  -> RRF(60) Top 20 -> BGE rerank -> final Top 5 under 600/900 tokens
  -> hash/ID/revision/rank/count/token diagnostics only
```

`MEMORY_HYBRID_SHADOW_ENABLED=false` gates both Worker embedding claims and
API comparison Provider calls. Projection maintenance remains transactional
and independent of flags. A rerank result is recorded only after every
submitted ID/revision is reauthorized against the current user, scope,
Sensitive switch, time, epoch, generation, and canonical hash. Query embedding,
rerank documents, and Worker embedding bodies reuse the deterministic Memory
secret redactor. SQL hashes and lease fences retain the raw authoritative value;
if redaction removes the entire Provider input, that Provider lane makes zero
network calls and records only a bounded `SECRET_REDACTED`/terminal code.

`MEMORY_TOOL_LOOP_ENABLED=false` independently gates the API-only product
`search_memory` Tool. When explicitly enabled on an eligible Tool-capable
turn, no Memory body enters the first round. One exact call runs the fixed
hybrid path without v1 fallback, records the final set, hydrates it through
migration `065`, repeats current authority and secret redaction, then returns a
bounded Tool Result for same-model continuation. The Worker never receives this
flag; ready embeddings still depend on the shared hybrid Worker switch.

Only the current user message may force that first call. Explicit bilingual
saved-Memory commands and direct personal recall questions order
`search_memory` first and use named `required`. This includes bounded
first-person preference questions such as `我喜欢喝什么？` and
`what do I like to drink?`, but excludes other-subject questions, advice, and
quoted writing tasks. Ordinary tasks and general questions about memory remain
Auto. Capability discovery stays background and fail-closed. Official DeepSeek
Tool rounds/continuations use
`thinking.type=disabled` with no `reasoning_effort`, while plain no-Tool chat
retains its selected reasoning behavior. This entry policy never replaces the
fixed BGE/Luna candidate release boundary.

Migration `070` makes deployment state explicit. The Worker must establish and
refresh a PostgreSQL heartbeat before processing lanes and retires it on clean
shutdown. `memory_user_health(UUID)` combines live extraction/embedding worker
capability with only the authenticated user's capture and current-authority
projection counts. Settings display this bounded state persistently. A
candidate-less Memory Tool result is a normal miss only while health is
`ready`; `indexing`, `degraded`, `disabled`, or unreadable health becomes an
explicit failed Tool step and same-model continuation without Memory. Existing
non-empty, fully reauthorized final results may still be released during a
subsequent worker outage.

Official DeepSeek may still synthesize a `query` field for that zero-argument
Tool. The Provider adapter discards every member of a bounded valid JSON object
only when the server declared the function zero-argument, then passes canonical
`{}` to validation and continuation. It never uses model-generated query text;
malformed/oversized input and non-zero-argument Tools retain their normal
validation, and the canonical Memory Tool hash stays unchanged.

## Rollback

Migration `053` down is allowed only before v2 authority is used. It fails atomically when any
Project exists, any Memory is non-Global, Conversation policy/generation has
changed, or Sensitive/L2/L3 settings differ from their migration defaults.

Migration `053` replaces the unique index used by the pre-`053` backend's
Memory create statement. Therefore an old and new backend must not run
together after `053`. Forward deployment has an explicit short outage:

```text
stop every pre-053 backend
  -> up migration 053
  -> deploy the post-053 backend
```

If a pre-traffic schema rollback is approved:

```text
stop every post-053 backend
  -> verify down guards
  -> down migration 053
  -> deploy the pre-053 backend
```

After v2 data exists, keep the additive schema and roll back only later feature
flags/readers. Never delete Projects or scoped Memory merely to make down pass.

Migrations `054`–`059` add their own history/queue guards. In particular,
`057` cannot roll back while any direct action, normalized target, Activity,
Usage, typed prior snapshot, or `direct_user` canonical row exists. Prefer a
forward fix after traffic. A clean pre-traffic drill must replay
`056 -> 057 -> 056 -> 057` against disposable PostgreSQL 17.

Migration `059` down also requires a v1/NULL reader and no hybrid observation
history. The vector projection and embedding jobs are rebuildable, but
observation evidence is not disposable. After shadow collection begins,
disable the shared flag and retain the schema instead of forcing down.

Migration `070` down additionally requires all live Worker heartbeats to be
retired or expired. Stop the Worker first; never delete a heartbeat row through
a runtime role. Clean rollback restores the prior readiness capability and
re-up recreates only derived health state.
