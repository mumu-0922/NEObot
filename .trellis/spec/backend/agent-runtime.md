# Agent Runtime Backend Contract

## Scenario: Durable package-Skill execution

### 1. Scope / Trigger

Apply this contract for Agent Skill admission, Run/Step/Attempt persistence,
Capability Grants, Runner RPC, side effects, Child Agents, Cron, Draft learning,
  Kill Switches, or the legacy text-Skill cutover. G20.1 implements the
  no-execute supply chain and G20.2 implements the internal durable
  Orchestrator control plane; current Chat, MCP and `/v1/code/executions`
  behavior remains unchanged.

### 2. Signatures

- Architecture: `mm-chat/docs/architecture/agent-runtime.md`.
- Contract: `mm-chat/docs/contracts/agent-runtime.md`.
- Schemas: `mm-chat/docs/contracts/schemas/neo-*.schema.json`.
- Fixtures: `mm-chat/docs/contracts/fixtures/agent-runtime/`.
- Offline gate: `bash mm-chat/scripts/verify-agent-runtime-phase0.sh`.
- Epic slices: `mm-chat/docs/tracking/g20-agent-runtime-plan.md`.

### 3. Contracts

- Assistant, Skill and Tool stay separate. Skill install never enables a Tool,
  credential, model, Assistant or permission.
- PostgreSQL is durable authority for Run/Step/Attempt, events, leases,
  snapshots, approvals, Cron revisions, admissions and Kill Switches. Redis is
  at most an ID-only wake/cancel hint.
- G20.2 signatures are `internal/agentorchestrator`, migration `084`, and
  `scripts/verify-agent-orchestrator{,-postgres17}.sh`. The package has no
  HTTP/startup import; `go_api_runtime` has no G20.2 privileges. The dedicated
  Runtime role has SELECT plus exact function execution and no table DML.
- Adding any new tail migration requires advancing every PostgreSQL drill that
  peels older tails (`verify-mcp-postgres17.sh`, MCP credential, Assistant
  Store, Skill supply, and the owning new drill). Each must down the new empty
  tail before asserting an older migration guard, then reapply through the
  current head; otherwise the drill may test or report the wrong migration.
- Every Run freezes model, budgets, Workspace, lineage, admitted package/runtime
  fingerprints, Tool Registry, Capability Grant, Egress and Secret refs.
- Runner RPC includes exact snapshot and lease generation. Reclaimed Attempts
  cannot publish output, Artifacts, Prepare or Commit.
- Package/manifest/model/Tool output is untrusted and never creates authority.
  Agent Skills `allowed-tools` is declarative only.
- Immutable Skill package replay compares decoded `allowed_tools` and
  `capability_requests` structures, not their original JSON bytes: PostgreSQL
  `jsonb` normalizes object representation, so byte comparison would falsely
  report `SKILL_PACKAGE_COLLISION` for an idempotent candidate.
- Mutable/external actions use Prepare/Commit. Missing acknowledgement after a
  possible effect is terminal `outcome_unknown`; never blind-retry the Commit.
- Root Run depth is `0`; Child is exactly `1`. Child Registry construction
  physically removes `delegate_task`, Cron/grant/secret/runtime management
  before fingerprinting. Child snapshots/budgets are strict subsets.
- Cron freezes authorization, model, budgets, fingerprints, Egress, Secrets and
  schedule/timezone, then rechecks current revoke/expiry/Kill Switch per trigger.
- Learning outputs quarantined Drafts only; human Promote creates a new admitted
  fingerprint and never mutates live/installed/Cron snapshots.
- Final cutover hard-deletes legacy text-Skill definitions/selections/execution
  paths without migration/wrapping. Historical messages retain only the
  read-only fact “旧版技能已退役”. Phase 0 must not delete them early.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| missing/drifted rootless isolation evidence | `ISOLATION_UNAVAILABLE`; no lease/launch |
| package/runtime/grant/snapshot mismatch | `SNAPSHOT_MISMATCH`; new Run required |
| stale lease generation | `LEASE_STALE`; no output/Artifact/effect |
| action outside Grant or current policy | `GRANT_DENIED`; no Broker call |
| applicable Kill Switch | `KILL_SWITCH_ACTIVE`; fence mutable effects |
| ambiguous external result | terminal `OUTCOME_UNKNOWN`; operator reconcile |
| Child depth 2 or forbidden registry Tool | reject before launch |
| Draft attempts self-Promote | reject and security-audit |

### 5. Good / Base / Bad Cases

- **Good**: exact admitted package + frozen Grant creates a leased Attempt,
  non-root Runner starts a rootless Sandbox, Broker mediates I/O, PostgreSQL
  appends terminal facts and cleanup reaps all state.
- **Base**: all Runtime switches off; Chat/current text Skills continue and
  cleanup/reconciliation still runs after later persistence exists.
- **Bad**: browser assembles executable Skill context, model expands Tools,
  Backend runs code in-process, Runner uses rootful Docker, Child retains
  `delegate_task`, or a write reconnect is automatically retried.

### 6. Tests Required

- Phase 0: schema check, positive/negative fixtures, cross-contract invariants,
  Markdown anchors/links and current code-execution fail-closed proof.
- Supply chain: malicious archive/source drift/fingerprint/SBOM/admission corpus.
- Supply-chain PostgreSQL replay must insert the same candidate twice through
  migration `083` and prove normalized `jsonb` metadata remains idempotent.
- G20.1 signatures: `internal/skillsupply`, migration `083`,
  `/v1/skills/candidates|store|library`, and
  `scripts/verify-skill-supply-chain{,-postgres17}.sh`.
- Candidate `neo.runtime.json` contains declarations only. Source/admission and
  package/runtime/SBOM fingerprint bindings are server-generated envelope
  fields; complete manifest bytes remain inside package identity.
- Durable state: PostgreSQL replay/down/up, state/race/lease/restart/projection,
  least privilege, backup/restore and retention.
- G20.2 PostgreSQL replay includes idempotent enqueue collision, first-terminal
  concurrency, gap-free sequence, exact token-digest/generation heartbeat,
  reclaim/stale denial, restart inventory, projection rebuild, Kill Switch
  revisions/hierarchy, terminal retention and content-free dump/restore.
- Runner: exact target-host Isolation Acceptance Suite, resource/escape/network/
  secret/kill/orphan/reboot negatives.
- Side effects: Prepare/Commit crash/acknowledgement-loss/idempotency matrix.
- Child: forged Parent, depth 2, widened grant/model/package/budget and registry
  alias rejection before launch.
- Cutover: backup, storage purge, zero legacy execution references, history fact,
  clean-copy/restart/live canary and all-path rollback rehearsal.

### 7. Wrong vs Correct

#### Wrong

```text
SKILL.md says allowed-tools -> model sees delegate_task -> child creates child
```

#### Correct

```text
package request -> server Grant intersection -> depth-1 forbidden-set removal
-> registry fingerprint -> launch admission -> no delegate_task exists
```
