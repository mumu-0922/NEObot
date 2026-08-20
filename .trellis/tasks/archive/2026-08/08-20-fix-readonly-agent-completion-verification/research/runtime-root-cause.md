# Read-only Agent Completion Failure — Runtime Root Cause

## Reproduction evidence

User prompt:

```text
在当前项目执行 pwd 和 git status --short，然后解释结果，不要修改文件。
```

Observed live run:

- Conversation: `6eb750e8-ab96-4c68-9c30-e7e9d0bfa541`
- Assistant message: `bffeb37e-d9ee-4af4-8b72-34cf49519465`
- Run: `1073af86-1995-44da-af34-976472133a41`
- Turn: `d837bf73-1f23-4f62-9430-b9a9fc5c820e`
- Six Terminal calls completed.
- Two `verify_completion` calls failed with
  `failureCategory=verification_evidence_invalid`.
- The Turn stopped with `AGENT_VERIFICATION_REQUIRED` after 53.15 seconds.

## Executing path

1. `chat_tool_registry.go` registers every `terminal` call as
   `RiskClass=execute`.
2. `chat_completion_policy.go::observe` treats every successful `write` or
   `execute` registration as the latest mutation.
3. `web_tool_loop.go` buffers narration while that mutation is outstanding and
   ultimately maps exhausted rounds/budgets to `AGENT_VERIFICATION_REQUIRED`.
4. `localSkillSuccessResult` serializes Tool output without an explicit
   `evidenceToolCallId`, although `verify_completion` requires the exact
   Provider Tool Call ID.

The live behavior therefore matches the currently served source contract. The
failure is a Completion Policy defect, not a missing Terminal runtime.

## Options considered

### A. Parse Shell commands into read/write classes

Rejected. Pipes, redirections, command substitution, aliases, scripts and
language runtimes make semantic classification incomplete and bypass-prone.

### B. Treat all Terminal calls as non-mutating

Incomplete. A background start must not be considered finished while its
process is still running.

### C. Foreground boundary plus background completion contract (selected)

- A successful foreground Terminal does not create outstanding mutation.
- A background Terminal start still creates outstanding mutation by exact Job
  ID; multiple starts remain independently pending.
- Only same-Job `job_output(status=completed)` can close that asynchronous
  boundary; foreground Terminal and unrelated Job output cannot.
- Structured write Tools remain evidence-gated.
- Foreground Terminal remains usable as later evidence for a structured write.
- Local successful Results disclose their own exact provider call ID to the
  same model as `evidenceToolCallId`.

This changes only completion bookkeeping. Registry risk classification stays
`execute`, so ordering, approval, Process presentation and operational safety
remain unchanged.

## Applicable contracts

- `.trellis/spec/backend/chat-tool-loop.md`
- `.trellis/spec/backend/agent-runtime.md`
- `.trellis/spec/backend/mcp-tools.md`
- `.trellis/spec/guides/code-reuse-thinking-guide.md`
- `mm-chat/docs/contracts/chat-tool-loop.md`
- `mm-chat/docs/contracts/local-skill-runtime.md`

## Rollout and rollback

Build and replace only the Backend image. Keep the previous Backend image tag,
the live mode-0600 environment file and all current runtime volumes. Rollback
means restoring the previous Backend image and recreating only the Backend
container; no schema or data rollback is involved.
