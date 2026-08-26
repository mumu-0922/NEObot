# Resource Orchestrator Design

## Goals

- Give conversational acquisition and the Agent one Resource contract.
- Preserve Skill admission and MCP grant/credential/validation authorities.
- Bind writes to immutable candidate identity or current CAS revision.
- Keep credentials and untrusted Marketplace content outside model context.
- Refresh resources only at an explicit Run-segment boundary.

## Non-goals

- Executing arbitrary Pi Extensions, npm packages, Git repositories, or shell
  installers in the API process.
- Replacing Skill supply-chain review, MCP configuration UI, or vault storage.
- Mutating the active Agent Tool list in place.

## Architecture

```text
Chat acquisition / Agent Tools
        |
        v
strict Handler / Service
   | catalog + search
   | exact install --------> skillsupply / mcpclient
   | lifecycle mutation ---> skillsupply / mcpclient
   | config completion ----> MCP provenance + ready + credential
        |
        +--> audit_logs (metadata only)
        +--> refreshRequired -> fresh Runtime Resource Snapshot
```

`service.go` owns shared DTOs and read projections. `install.go` owns exact
candidate installation and the configuration completion protocol.
`mutation.go` owns lifecycle writes, shared CAS helpers, and audit finalization.
This split keeps each authority path independently reviewable and prevents a
single orchestration file from becoming a second domain service.

## Key decisions

| Date | Decision | Reason | Effect |
| --- | --- | --- | --- |
| 2026-08-25 | Delegate writes to existing domain services | Avoid bypassing admission, owner, grant, validation, and vault rules | The orchestrator contains no installer or credential store |
| 2026-08-26 | Exact revision plus bounded discovery | Marketplace metadata is mutable and untrusted | Stale candidates fail closed |
| 2026-08-26 | Action-specific mutation audit | Slash and Agent writes need one operator trail | `resource.install/enable/disable/remove` contain safe metadata only |
| 2026-08-26 | Configuration is a two-step install | Secrets must never enter Tool arguments | A provenance-bound draft is configured in Tools, then revalidated before continuation |
| 2026-08-26 | Fresh snapshot after mutation | Provider Tool definitions are frozen per Run segment | The original task continues only after a new revision is prepared |
| 2026-08-26 | Install changes inventory only | Persistent selection must remain explicit and Conversation-scoped | Continuation may use bounded `agent_auto`; only the composer picker writes selection |

## Security model

Threats include prompt-injected package instructions, candidate drift, forged
resource IDs, cross-user removal, selection races, credential leakage, and a
model claiming success before configuration or refresh.

Controls:

- Strict JSON and Tool schemas reject unknown fields, including Secret-shaped
  payloads; errors never echo input.
- Search is bounded and candidate metadata is explicitly untrusted.
- Skill fingerprint, MCP deployment hash, owner/`CanManage`, readiness,
  credential presence, and selection revision are revalidated server-side.
- The model cannot call mutation HTTP APIs or supply a private draft ID to the
  configuration completion method.
- Audit metadata excludes Secrets, endpoints, raw package content, Tool
  arguments, and host/cache paths.
- Audit or snapshot-refresh failure stops success reporting and continuation.

## Known limits

- Configuration continuation uses the existing bounded Chat Agent approval
  wait. A Backend restart marks pending approval `restart_denied`; it does not
  reconstruct a Provider loop without its original execution context.
- Live smoke requires a real admitted Skill and an operator-configured MCP
  Marketplace account; automated fixtures validate the same authority path but
  do not replace that deployment check.

## Change history

### 2026-08-26

- Added conversational discovery/install, lifecycle mutations, configuration
  handoff, action-specific audit, and fresh-snapshot continuation.
