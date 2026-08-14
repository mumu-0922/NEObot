# G21.2 reviewed read-only action research

## Existing seams and constraints

- `agentbroker.RegistryToolFor` already binds Tool identity, capability,
  action, resource selector, approval class, call budget, classification and
  idempotency to a frozen Grant and Registry.
- `ReadOnlyExecutor` is deliberately distinct from `EffectExecutor`, but the
  production `Service.Commit` path currently consults only effect executors.
- Migration `086` terminalizes Attempt, Step and Run for every completed
  Commit. One Attempt therefore cannot safely execute a sequence of multiple
  terminal actions without changing a previously verified state machine.
- The Runner Prepare message contains Grant/Registry fingerprints, not their
  bodies. A relay must not treat fields supplied by the Sandbox or RPC caller
  as the authoritative Grant or Registry.
- Conversation MCP rows are not Agent effect authority. `MCPExecutor` requires
  an already-authorized committer and deliberately avoids that legacy source.

## Recommended canary model

Use one strict synthetic Run and one Tool Registry per terminal action. This
preserves migration `086` exactly and avoids a new multi-effect Step state.
The mounted plan is the only source for the complete Grant/Registry documents;
the relay requires their fingerprints and every request field to match it.

The first production slice registers only these reviewed identities/actions:

- `project.read` / `read_file`: bounded UTF-8 read from the exact read-only
  canary Project root, with traversal, link, special-file, case/Unicode and
  byte-limit rejection;
- `workspace.read` / `read_file`: the same confinement against the exact
  immutable canary Workspace snapshot root and expected fingerprint;
- `mcp.read` / one exact allowlisted Tool: auth-none, read-classified,
  idempotent MCP invocation through the existing private MCP Runner connector;
- `artifact.publish` / `publish`: a bounded publication executor, classified
  separately from mutable Project effects and constrained by the frozen
  Artifact policy.

All reviewed read tools require `classification=read`, `idempotent=true` and
`approval=automatic`. `agentbroker.Service` should route registered read
identities through `ReadOnlyExecutor`, fingerprint the canonical sanitized
result as the receipt, and never expose raw result bytes in logs, events or
durable Broker rows. An identity registered as read must reject any Registry
that reclassifies or widens it.

## MCP boundary

The canary service may hold the private MCP Runner bearer token; the Sandbox
may not. The plan pins one manifest server and one read-only Tool with no
credential environment. Generic marketplace/private servers, OAuth/header/env
credentials, unknown classification and arbitrary Tool names remain denied.

The existing MCP Runner protocol can execute this reviewed manifest Tool
without using Conversation execution rows. A small Agent-specific committer
keeps the exact idempotency key and bounded sanitized result. If dispatch may
have occurred and status cannot prove committed or rejected, Commit ends as
`outcome_unknown`.

## Replay and retry

- Prepare replay must return the same intent for the same request fingerprint
  and reject a collision.
- Commit replay uses the same intent and idempotency key and returns the
  terminal receipt without dispatching again.
- Read retry is eligible only before any result was observed and only for the
  exact read/idempotent/automatic Registry entry.
- The G21.2 canary intentionally exercises an acknowledgement-loss read path
  and proves it becomes `outcome_unknown` with no automatic second dispatch.
- Mutable/unknown Tools, Project writes, generic Egress, Secrets, Provider,
  Child, Cron and Learning remain disabled.
