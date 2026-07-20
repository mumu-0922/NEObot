# Server Conversation Context Contract

## 1. Scope / Trigger

This contract applies whenever Go builds Provider input for an assistant stream.
It covers current-branch replay, token budgeting, derived rolling summaries,
Postgres persistence, and degradation. It does not define long-term user memory
or a response cache.

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
