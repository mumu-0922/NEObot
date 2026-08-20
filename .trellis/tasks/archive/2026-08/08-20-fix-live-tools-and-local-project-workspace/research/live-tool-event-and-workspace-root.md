# Live Tool event and local workspace root findings

## Decisive runtime evidence

- A captured successful Backend stream emits `tool.call.updated` with
  `executionId`, `toolName`, `processStatus`, `status`, `mode`, `round`, and
  classification, but no provider `callId`.
- Local File mutations legitimately use `classification=write`; Terminal uses
  `classification=execute`.
- The Frontend currently requires `callId` and allows only read/execute for
  `local_direct`, so a live browser rejects the event even though detached
  Backend execution continues. Reload can later render the durable Agent event
  timeline, which explains why the earlier API acceptance looked healthy.

## Workspace alternatives considered

### A. Mount all projects

Rejected. It would expose Neo Chat runtime configuration, backups, Secrets and
unrelated projects to ordinary Agent Tools.

### B. Keep the private data workspace and require manual copies

Rejected for this task. It preserves the narrowest boundary but does not meet
the requested local coding-Agent workflow.

### C. One explicit authorized project root with input aliases

Selected. Reuse the existing single bind and `os.Root` authority, mount only
`Oncall_Agent`, and configure its canonical Host path as a non-authoritative
input alias. Convert Linux absolute and WSL UNC representations into a relative
path before the existing traversal/symlink/bounded-I/O checks.

## Contract implications

- Missing live `callId` is a compatibility case, not permission to accept an
  unbounded or missing identity: `executionId` remains mandatory and bounded.
- Alias normalization must never create a second filesystem authority. A path
  outside the configured Host root fails before filesystem access.
- Terminal command text is opaque shell syntax and must not be rewritten.
  Only its structured `workingDir` can use the alias resolver; the prompt tells
  the model to execute relative to `$PWD`.
- Default installations keep the existing `./data/agent-workspace` source and
  omit Host aliases. Live rollout can be reversed by restoring the protected
  env and retained images; old workspace bytes are never deleted.
