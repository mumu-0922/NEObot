# Chat Agent Runtime Contract

## Scenario: execute installed Skills through the ordinary Chat Agent

### Scope / trigger

Apply when changing `internal/chat`, `internal/localskills`,
`internal/skillsupply`, `/v1/skills/*`, workspace Files/Jobs/Terminal,
artifact publication, or migration `098`.

### Signatures

```text
Conversation config: toolMode = "chat" | "agent"
Skill API: /v1/skills/*
Agent local Tools: skill, file_read, file_write, file_edit, file_search,
                   terminal, job_list, job_output, job_kill, publish_file
Migration head: 099_chat_agent_event_log_function_repair
```

### Contracts

- Agent is a persisted Conversation runtime policy, not a separate control
  application. Chat mode physically omits Agent Tools; Agent mode adds Skills,
  File, Terminal, Job, Browser/MCP, Goal, and `publish_file`.
- Unsupported Tool models downgrade the effective Turn to Chat without changing
  stored intent or returning Skill/MCP admission conflicts.
- Keep `internal/agents` (Assistant library), `internal/localskills`, and
  `internal/skillsupply`. The Skill Store remains server-authoritative through
  `/v1/skills/*` and must not regain Runs, Schedules, Learning, Shadow, Canary,
  Runner, OCI, delegation, or Subagent controls.
- `local_direct` runs as the Backend UID/GID under two explicit roots. Enforce
  timeout, output, call, round, and concurrency bounds plus process-group
  cancellation/reaping. Jobs remain process-local and must never claim restart
  durability.
- `AGENT_LOCAL_WORKSPACE_HOST_ROOT`, when nonempty, must be the clean absolute
  Host path for the one directory already mounted at `/workspace`. Resolve only
  canonical Linux absolute and WSL UNC inputs below it into relative names
  before the existing `os.Root`/CAS/symlink checks. It is never a second root.
- Local execution is not a Sandbox. Never add `sudo`, a container socket,
  host-wide personal/secret binds, privileged execution, automatic OS package
  installation, or per-Skill isolation claims.
- Foreground Terminal success is a synchronous execution boundary and does not
  create a Completion Policy mutation. Do not classify arbitrary Shell text as
  read-only/write. Structured File/MCP writes remain evidence-gated; foreground
  Terminal may check them. Background Terminal remains outstanding by exact Job
  ID until successful `job_output(status=completed)` evidence is explicitly
  recorded. Successful local Tool Results expose their exact Provider-only
  `evidenceToolCallId`; Process events remain content-free.
- `publish_file` accepts only workspace-relative regular files, persists through
  the existing user-owned File/object-store path, and attaches only successful
  outputs to the assistant message. Cross-user and stale/deleted access fails.
- Applied migration SQL is byte-immutable. The production checksum for `096`
  is `f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042`;
  runtime corrections belong in forward migration `099`, never in the old
  pair or `schema_migrations.checksum`.
- Migrations `084`–`095` remain immutable history. Migration `098` uses explicit
  object whitelists, locks before counting, permits only two bootstrap
  singleton rows, aborts on any fact row or schema drift, uses no wildcard
  deletion/`CASCADE`, and never names `chat_agent_*`.
- `098.down` is an irreversible no-op. Reapplying after down must succeed only
  when every retired object is already absent.
- `099` idempotently repairs the two Chat event gateways, reasserts hardened
  `search_path` and least-privilege grants, and has a forward-only no-op down.

### Validation matrix

| Condition | Required result |
| --- | --- |
| Chat mode or Tool-incapable model | no local/MCP/Goal Tool preparation |
| Agent enabled, no installed Skills | File/Terminal/Job remain; Skill catalog is empty |
| invalid root/shell/limits | Backend startup fails |
| destructive command in smart mode | approval-required result; no execution |
| foreground Terminal-only task | Tool-free final answer; no `verify_completion` loop |
| background Terminal plus foreground check | background remains unverified until exact completed `job_output` |
| Backend shutdown with active Job | entire process group canceled and reaped |
| path traversal/symlink/non-regular publish | reject; no File row/object |
| Host/WSL alias below configured workspace | resolve to the same relative File/workingDir path |
| alias outside Host root, Windows drive, or traversal | reject before filesystem/command access |
| nonempty legacy fact table at 098 | whole migration rolls back |
| already-retired schema re-up | successful no-op |

### Good / base / bad cases

- **Good**: Agent mode loads an admitted Skill, maps an authorized pasted Host
  path to a workspace-relative name, edits that file, verifies it, publishes
  it, and returns an authenticated download card.
- **Base**: Agent mode has no installed Skills; bounded File/Terminal/Job Tools
  still work, while Chat mode exposes none of them.
- **Base**: Agent runs `pwd` and `git status --short` in foreground, observes the
  redacted workspace result, and answers without manufacturing verification.
- **Bad**: route local execution through a second control service, claim Sandbox
  isolation, parse arbitrary Shell text as a reliable mutation classifier,
  verify a running background Job with unrelated foreground output, mount a
  Home/parent directory containing unrelated secrets, treat an alias as a
  second root, or let migration `098` delete by wildcard.

### Required tests

```bash
bash mm-chat/scripts/verify-agent-local-runtime.sh
bash mm-chat/scripts/verify-legacy-agent-cleanup-postgres17.sh
bash mm-chat/scripts/verify-chat-artifacts-postgres17.sh

cd mm-chat/backend
GOCACHE=/tmp/neo-chat-go-cache go vet ./...
GOCACHE=/tmp/neo-chat-go-cache go test ./...
```

The local Runtime suite must also prove foreground Terminal-only completion,
`file_write -> terminal -> verify_completion`, exact local
`evidenceToolCallId`, and background Job ID/status gating.

Cross-layer changes also require frontend format/lint/typecheck/test/build and
`bash mm-chat/scripts/verify-standalone.sh --full`.

### Wrong vs correct

```text
Wrong: Agent Center -> OCI Runner -> Broker -> downloadable host path
Correct: Chat Agent -> bounded local Tool -> user-owned File -> message artifact

Wrong: inspect Shell command text -> guess mutation -> force verification
Correct: foreground result -> synchronous boundary; background Job -> exact completed output

Wrong: DROP ... CASCADE after a broad agent_* match
Correct: lock -> exact manifest/data validation -> explicit drops -> forward repair -> head 099
```
