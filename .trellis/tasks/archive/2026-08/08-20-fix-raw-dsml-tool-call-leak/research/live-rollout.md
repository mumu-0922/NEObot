# Live Backend rollout

## Root-cause evidence

- Persisted assistant message: `8de257bc-02c9-4974-9581-230dc27b4d8d`
- Provider/model: `FOHWSU/deepseek-v4-flash`
- Persisted mode: `requestedToolMode=chat`, `toolMode=chat`
- The first native Tool Call was the allowed `search_memory`; it failed with
  `memory_service_unavailable`.
- The following Provider `content` delta contained the fullwidth-bar DSML
  envelope and `exec_command`. `output_blocks` was empty, while both
  `messages.content` and durable `assistant.message` contained the same text.
- No Terminal event or execution occurred.

This proves a Provider-content boundary failure rather than a Frontend-only
Markdown issue. The raw envelope remains untrusted text and is never converted
into an executable Tool Call.

## Focused verification

```text
go test ./internal/chat -run '^(TestOpenAICompatible|TestDeepSeekCompatible)' -count=1
go vet ./internal/chat
git diff --check
```

All passed. Regression coverage includes fragmented fullwidth and ASCII DSML
markers, safe-prefix preservation, typed failure, absence of raw Tool names in
visible deltas/errors, ordinary content, and native structured Tool Calls.

## Protected rollout

- Source commit: `af89f7af`
- Previous image:
  `mm-chat/backend:terminal-completion-e7ba11f2fed4-20260820T072454Z`
- Previous image ID:
  `sha256:18195df26ee7108e9d971f54ecec898f242e8beda05017f998a0b57d23f10fb0`
- Rollback directory:
  `mm-chat/backup/raw-dsml-guard-af89f7af-20260820T082212Z`
- Environment backup and rollout evidence retain mode `0600`; the directory
  retains mode `0700`.
- No migration, persistent-data rewrite, Frontend rebuild, or historical
  message cleanup was performed.

The Backend image was built with the required Dockerfile `runtime` target and
inspected before activation:

```text
image=mm-chat/backend:dsml-guard-af89f7af-20260820T082212Z
image_id=sha256:908ccf9386e5411929df19125a3a987ce8e45072a609b92bb7046a62c5cec2cf
user=mmchat:mmchat
cmd=["/usr/local/bin/mm-chat-api"]
```

Only `backend` was force-recreated. The new container
`397b5f1999af` is healthy, `/ready` reports database/Redis/storage ready, and
migration head remains `99`. The before/after Compose container-ID set differs
only by the old/new Backend IDs. Frontend, MCP Runner, MinIO, PostgreSQL, and
Redis container IDs remain unchanged.

## Rollback

Restore the protected `.env.single-server.before`, keep it mode `0600`, then
force-recreate only `backend`. The retained previous image remains locally
available. No database rollback is required.

## Remaining boundary

The already-persisted failing message and immutable agent event are retained
for diagnosis and are not rewritten. A new response is required to exercise
the deployed guard; a stochastic live Provider replay was not treated as
deterministic acceptance evidence.
