# G11.13 Conversation Context and Memory Process

## 2026-07-20 — G11.13A current branch replay

### Root cause

Postgres already persisted conversation messages, but `ProviderRequest` exposed
only the current `Prompt`. Both OpenAI-compatible Chat Completions and OpenAI
Responses built-in Search therefore sent `system + current user`. Ordinary
frontend sends also omitted `parentMessageId`; the UI reconstructed a linear
tree by sequence while the stored parent chain remained incomplete.

### Closure

- Added ordered provider-neutral messages and effective branch reconstruction.
- Kept explicit frontend branch metadata authoritative, admitted old
  `parent_message_id = NULL` rows through the same active-path behavior as the
  frontend, and preserved explicit null as a new root.
- Wired the ordered path into ordinary Chat Completions and Responses Web
  Search payloads.
- Applied final Knowledge/Web prompt augmentation only to the current user item.
- Made ordinary browser follow-ups persist the current active leaf.

No schema migration, message rewrite, idle compaction, summary, long-term
memory, or historical attachment rehydration was included.

### Verification

Automated gates:

```text
backend go test ./...                         passed
backend go vet ./...                          passed
frontend lint                                 passed
frontend typecheck                            passed
frontend tests                                180 files / 866 tests passed
frontend production build                     passed
```

Coverage includes linear legacy history, selected sibling branches, explicit
roots, final grounded-prompt replacement, Handler-to-Provider flow, both
provider payload families, and frontend active-leaf wiring.

Live proof rebuilt the backend from the current standalone source, used the
existing Postgres-backed Server Default with real `gpt-5.6-sol`, completed a
first turn containing a one-time marker, restarted the backend, and completed a
second turn that recalled the marker. The user follow-up retained the assistant
parent, four messages reloaded from Postgres, readiness returned healthy after
restart, and both SSE runs completed. The marker, row IDs, credentials, stream
captures, and disposable conversation were not retained.

### Rollback

Revert the slice commit and rebuild the backend/frontend. No database rollback
is required; newly persisted parent IDs remain valid under the former runtime.

## 2026-07-20 — G11.13B token-budget soft consolidation

### Contract

The server owns model context windows and budgeting. It reserves output and
estimation headroom, triggers at 80% of the input budget, and targets 50%.
Provider input keeps a recent user-boundary raw tail and injects a generated
assistant summary under an explicit lower-priority/untrusted-history system
instruction. Browser requests cannot raise context limits.

Migration `034` adds `conversation_context_summaries`. Each row records a
monotonic version, source first/last message IDs, source count, SHA-256 digest
over exact source IDs/roles/content, generation model, summary, estimates, and
timestamps. Original `messages` rows are never changed or deleted by
consolidation. Digest mismatch from edits or branch switches disables reuse.

Summary prompts serialize untrusted history as JSON and forbid following its
instructions. Rolling consolidation combines the prior summary only with newly
evicted messages. Empty/error/oversize summary output, missing persistence,
unsafe boundaries, or read failure produces a deterministic recent-tail
fallback and a bounded diagnostic code.

### Verification

Automated coverage proves model-prefix resolution, conservative multilingual
estimation, boundary selection, exact prefix invalidation, initial and rolling
summary versions, restart reuse without another summarizer call, Handler
payload/metadata, provider failure fallback, Service limits, schema fragments,
Repository round-trip, version increment, and foreign-conversation boundary
rejection.

```text
backend go test ./...                         passed
backend go vet ./...                          passed
backend chat + migration race tests           passed
frontend format/lint/typecheck                 passed
frontend tests                                180 files / 866 tests passed
frontend production build                     passed
backend Compose source build/readiness         passed
```

A disposable Postgres database applied all migrations through `034`, exercised
the Repository path plus `034` down/up replay, and was deleted. The formal stack migrated from 33 to 34
and rebuilt the backend from standalone source. A real `gpt-5.6-sol` run crossed
the high-water mark, summarized 20 messages into v1, and recovered a one-time
marker held only in the summarized prefix. After restarting Go, the next turn
reused v1 and recovered the marker again. Both SSE answers completed; all
fixture rows and captures were hard-deleted. Credentials, IDs, marker text, and
raw streams were not retained.

### Rollback

Roll back the backend first. The older backend ignores the derived table, so
leaving `034` applied is safe. If schema rollback is required after all newer
instances stop, run one migration down; only derived summaries are removed and
original messages remain untouched.
