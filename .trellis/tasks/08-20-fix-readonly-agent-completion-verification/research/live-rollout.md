# Live Backend rollout and acceptance

## Protected rollback state

- Rollout directory:
  `mm-chat/backup/agent-terminal-completion-e7ba11f2fed4-20260820T072156Z`
- Previous Backend image:
  `mm-chat/backend:live-tools-e0ce997005a9-20260820T064039Z`
- The pre-rollout live environment is retained mode `0600`.
- No database migration or persistent-data rewrite was required.

## Rejected first candidate and recovery

The first manual build omitted `--target runtime`. Because
`backend/Dockerfile` ends in the separate `mcp-runner` stage, the resulting
container had no API listener and failed its port-8080 health check. The
protected environment and previous Backend image were restored immediately;
the old Backend returned healthy before another build was attempted.

The corrected build selected `--target runtime` and its inspected config was:

```text
user=mmchat:mmchat
cmd=["/usr/local/bin/mm-chat-api"]
```

This operational gotcha is now recorded in the runtime image-pinning spec and
release runbook.

## Final deployed state

- Source commit: `e7ba11f2fed4`
- Backend image:
  `mm-chat/backend:terminal-completion-e7ba11f2fed4-20260820T072454Z`
- Backend image ID:
  `sha256:18195df26ee7108e9d971f54ecec898f242e8beda05017f998a0b57d23f10fb0`
- Backend container is healthy and `/ready` reports database, Redis and storage
  ready.
- Migration head remains `099`.
- Frontend, MCP Runner, MinIO, PostgreSQL and Redis image/container identities
  are unchanged.
- Host and container `/workspace/README.md` SHA-256 remain
  `ee0cf9cb2be3371b8b54045021d4379dbc29c6416138c97eb16f1e14428e4e2f`.

## Real Agent acceptance

Retained Conversation: `bf87b6a9-a89a-452a-9041-2ac841c4bab7`

Exact prompt:

```text
在当前项目执行 pwd 和 git status --short，然后解释结果，不要修改文件。
```

Observed proof:

- Assistant status: `completed`
- `requestedToolMode=agent`
- `toolMode=agent`
- Tool names: only `terminal`
- Tool updates: two (`running`, `completed`)
- Successful Terminal observed: yes
- `verify_completion` updates: zero
- SSE `message.error`: zero
- Persisted answer explained both Workspace and Git status.

The one-time operator acceptance Session was logged out and deleted. The
Conversation remains available to the owner. Raw SSE and sanitized result are
retained mode `0600` under the rollout directory and are not committed.
