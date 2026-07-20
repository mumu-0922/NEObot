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
