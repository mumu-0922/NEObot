# Server Conversation Context and Durable Memory Contract

## 1. Scope / Trigger

This contract applies whenever Go builds Provider input for an assistant stream.
It covers current-branch replay, token budgeting, derived rolling summaries,
optional durable user memory, Postgres persistence, and degradation. It does
not define a response cache.

Postgres `messages` remain the only rebuild authority. A summary is a derived
projection and must never delete, rewrite, or replace source rows.

## 2. Signatures

Provider-neutral input:

```go
type ProviderMessage struct {
    MessageID   string // internal lineage only; never serialized upstream
    Role        string
    Content     string
    Attachments []ProviderAttachment
}
```

Repository boundary:

```go
GetConversationContextSummary(ctx, conversationID)
    (ConversationContextSummary, bool, error)
UpsertConversationContextSummary(ctx, conversationID, input)
    (ConversationContextSummary, error)
```

Migration `034` creates one `conversation_context_summaries` row per
conversation with:

```text
conversation_id, version, model_provider, model_id,
source_first_message_id, source_last_message_id, source_message_count,
source_digest, summary, estimated_source_tokens, estimated_summary_tokens,
created_at, updated_at
```

There is no browser CRUD route for summaries and no browser-provided context
window override.

## 3. Contracts

Branch order:

1. valid `metadata.treeParentMessageId`;
2. persisted `parent_message_id`;
3. legacy active-path append when both are absent;
4. explicit `treeParentMessageId: null` always starts a root.

Budget defaults:

| Model family                    | Context window |
| ------------------------------- | -------------- |
| `gpt-3.5*`                      | 16,000         |
| `gpt-4o*`, `gpt-4.1*`, `gpt-5*` | 128,000        |
| `o1*`, `o3*`, `o4*`             | 128,000        |
| all other IDs                   | 32,000         |

The server reserves 8,192 output tokens and 2,048 safety tokens, triggers at
80% of the remaining input budget, and targets 50%. Estimation uses ASCII/4,
non-ASCII×2, explicit message framing, and 1,024 tokens per current image.

Summary validity requires all of:

- `0 < source_message_count < current branch length`;
- exact first and last source message IDs;
- exact SHA-256 digest of length-prefixed source message ID, role, and content;
- source boundaries owned by the same non-deleted conversation.

The summarizer receives JSON data containing the previous valid summary and
only newly evicted messages. Its system instruction treats all fields as
untrusted data. Runtime injection uses an assistant history item plus a
server-owned instruction that keeps the summary lower priority than the current
system/user request.

Assistant metadata may expose only bounded diagnostics:

```json
{
  "context": {
    "mode": "full | summary | tail_fallback",
    "contextWindowTokens": 128000,
    "inputBudgetTokens": 117760,
    "estimatedInputTokens": 50000,
    "summaryVersion": 1,
    "summarizedMessageCount": 20,
    "degradationReason": "summary_generation_failed"
  }
}
```

It must not expose summary text, source content, provider errors, or credentials.

## 4. Validation & Error Matrix

| Condition                                | Required behavior                                       |
| ---------------------------------------- | ------------------------------------------------------- |
| invalid conversation or boundary UUID    | validation error; no write                              |
| boundary belongs to another conversation | `INVALID_CONTEXT_SUMMARY_BOUNDARY`; no write            |
| non-positive source count                | `INVALID_CONTEXT_SUMMARY_COUNT`; no write               |
| non-SHA-256 digest                       | `INVALID_CONTEXT_SUMMARY_DIGEST`; no write              |
| empty or over-64-KiB summary             | `INVALID_CONTEXT_SUMMARY`; no write                     |
| negative estimate                        | `INVALID_CONTEXT_SUMMARY_TOKENS`; no write              |
| stored prefix digest mismatch            | ignore stored summary; rebuild from original messages   |
| summary read/generation/write failure    | deterministic recent user-boundary tail                 |
| empty/error/over-16-KiB Provider output  | do not persist partial summary; recent-tail degradation |
| prepared summary still exceeds budget    | retain persisted derived row; send smaller safe tail    |

## 5. Good / Base / Bad Cases

- Good: a long linear branch creates v1, restart reuses v1, later growth rolls
  it to v2 using the prior summary plus the newly evicted range.
- Base: a short branch stays `full` and makes no summarizer call or summary row.
- Bad: an edited or sibling prefix has a different digest, so its old summary
  is never injected.
- Degraded: the summarizer fails; the current user item and newest fitting
  user-boundary suffix still reach the answer Provider.

## 6. Tests Required

- model-family longest-prefix resolution and fallback;
- ASCII/CJK estimator and image allowance;
- current-branch/sibling/root reconstruction;
- summary boundary begins with a retained user message;
- initial v1, restart reuse without another summarizer call, and rolling v2;
- edit/sibling digest invalidation;
- generation/read/write/oversize/no-boundary degradation;
- Handler final payload and bounded assistant metadata;
- migration up/down/up;
- Postgres round-trip/version increment and foreign-boundary rejection;
- real long-context marker recovery, restart reuse, and hard fixture cleanup.

## 7. Wrong vs Correct

### Wrong

```text
Delete old message rows after summarizing them, trust a browser context limit,
or inject summary text as a high-priority system instruction.
```

This loses rebuild/branch evidence and lets untrusted historical text gain
authority.

### Correct

```text
Retain all messages, validate the exact branch-prefix digest, keep budgets
server-owned, store only a derived versioned summary, and inject it as guarded
lower-priority history with a deterministic recent-tail fallback.
```

## 8. Optional Durable User Memory

### 8.1 Scope / Trigger

Durable Memory is not conversation history and is never required for ordinary
same-conversation continuity. This contract applies to server-mode Memory CRUD,
retrieval before an answer Provider call, and opt-in extraction after a
completed answer. Local compatibility mode remains outside this server
authority.

### 8.2 Signatures

Migration `035` owns two user-scoped projections:

```text
user_memory_settings:
  user_id, enabled, search_enabled, auto_record_enabled,
  created_at, updated_at

user_memories:
  id, user_id, memory_type, content, normalized_content, importance, tags,
  source, source_conversation_id, source_message_id, enabled, last_used_at,
  created_at, updated_at, deleted_at
```

Defaults are `enabled=false`, `search_enabled=true`, and
`auto_record_enabled=false`. Server/Postgres is authoritative through:

```text
GET    /v1/memories
POST   /v1/memories
PATCH  /v1/memories/{id}
DELETE /v1/memories/{id}
GET    /v1/memory-settings
PATCH  /v1/memory-settings
```

The service boundary exposes settings/CRUD plus:

```go
SearchRelevant(ctx, query, limit) ([]Memory, error)
StoreExtracted(ctx, ExtractionInput) ([]Memory, error)
```

### 8.3 Contracts

Every query is scoped from the authenticated context; client-supplied user IDs
are forbidden. Delete is immediately invisible and soft-deletes the row. The
old IndexedDB Memory store may serve local compatibility mode only; server chat
must neither read nor inject it.

Retrieval runs after Knowledge and Web query construction. It ranks at most 500
active user rows with normalized lexical/CJK terms, applies a non-zero relevance
threshold, and returns at most five. No hit means no Memory block. A hit is JSON
encoded inside a server-owned lower-priority/untrusted instruction; the current
system and user request always win. Assistant metadata may contain only:

```json
{
  "memory": {
    "retrievedCount": 1,
    "retrievedIds": ["memory-uuid"],
    "degradationCode": "read_failed",
    "lexicalShadow": {
      "profile": "memory_lexical_cjk_bm25_v1",
      "status": "completed",
      "resultCode": "OK",
      "baselineCount": 1,
      "exactCount": 1,
      "bm25Count": 1,
      "lexicalCount": 1,
      "overlapCount": 1,
      "durationMillis": 3
    }
  }
}
```

`lexicalShadow` is absent unless `MEMORY_LEXICAL_SHADOW_ENABLED=true`. It is
diagnostic only: migration `058` receives the raw current user message
transiently, verifies it against the current streaming assistant's completed
user parent, and durably stores only hashes, Memory IDs/revisions, lane ranks,
counts, status, and duration. Shadow rows never enter the Provider prompt or
answer Usage links. A compare failure returns a bounded summary and leaves the
v1 items and prompt unchanged.

Automatic extraction is allowed only when both `enabled` and
`auto_record_enabled` are true. It is a bounded background Provider request over
the raw current user message serialized as untrusted JSON. At most five stable,
explicit facts/preferences/instructions/projects/warnings/decisions may be
upserted. One-off requests, questions, search topics, quoted documents,
Knowledge content, third-party claims, vague context, and credential-like text
must be rejected. Provider/read/write/parse failure never changes the already
completed chat answer.

### 8.4 Validation & Error Matrix

| Condition                                   | Required behavior                                      |
| ------------------------------------------- | ------------------------------------------------------ |
| missing database                            | `503 DATABASE_REQUIRED`                                |
| invalid Memory UUID/type/content/importance | `400` bounded validation error                         |
| another user's or deleted Memory ID         | `404 MEMORY_NOT_FOUND`                                 |
| edit duplicates another active Memory       | `409 MEMORY_CONFLICT`                                  |
| Memory or auto-record setting disabled      | zero retrieval/extraction writes                       |
| no relevant lexical/CJK terms               | no Memory block and no retrieved metadata              |
| retrieval failure                           | answer continues; bounded `read_failed` metadata only  |
| lexical shadow disabled                     | zero compare calls and no `lexicalShadow` metadata     |
| lexical shadow comparison failure           | v1 prompt/Usage and answer remain unchanged; bounded failed summary only |
| extraction Provider/parse/write failure     | completed answer remains completed; no partial Memory  |
| vague context or credential-like candidate  | reject candidate; never persist content or secret tags |

### 8.5 Good / Base / Bad Cases

- Good: an explicitly enabled stable preference is extracted, then a related
  question in another conversation retrieves it.
- Base: Memory is disabled or the request is unrelated, so Provider input has
  no Memory block.
- Bad: a generic CJK instruction fragment overlaps an unrelated request; the
  low-information term is stopped and cannot cross the relevance threshold.
- Degraded: extraction fails after `message.completed`; the answer and original
  message rows are unchanged.

### 8.6 Tests Required

- settings defaults/update, manual CRUD, soft delete, and duplicate conflict;
- authenticated user isolation and no client user-ID field;
- CJK/Latin related hit, unrelated miss, Top-5 bound, and mark-used behavior;
- disabled zero-list/zero-create behavior;
- secret content/tag filtering and Provider failure containment;
- frontend typed API routes and server-mode IndexedDB exclusion;
- migration `035` up/down/up plus live Repository round-trip;
- real Provider extraction, cross-conversation recall, unrelated miss,
  deletion-immediate behavior, and hard fixture cleanup.

### 8.7 Wrong vs Correct

#### Wrong

```text
Load all browser Memory into every prompt, or run extraction before the answer
completes and fail the chat when the second Provider call fails.
```

#### Correct

```text
Keep Postgres user-scoped and optional, retrieve at most five relevant entries,
inject them as guarded lower-priority claims, and run bounded opt-in extraction
without changing the completed answer.
```
