# Agent Runtime Backend Contract

## Scenario: Durable package-Skill execution

### 1. Scope / Trigger

Apply this contract for Agent Skill admission, Run/Step/Attempt persistence,
Capability Grants, Runner RPC, side effects, Child Agents, Cron, Draft learning,
  Kill Switches, or the legacy text-Skill cutover. G20.1 implements the
  no-execute supply chain, G20.2 the internal durable Orchestrator, G20.3 the
  held Runner, G20.4 the held Broker and G20.5 the held depth-1 delegation
  source/control foundations; current Agent API, Chat, MCP and
  `/v1/code/executions` behavior remains unchanged.

### 2. Signatures

- Architecture: `mm-chat/docs/architecture/agent-runtime.md`.
- Contract: `mm-chat/docs/contracts/agent-runtime.md`.
- Schemas: `mm-chat/docs/contracts/schemas/neo-*.schema.json`.
- Fixtures: `mm-chat/docs/contracts/fixtures/agent-runtime/`.
- Offline gate: `bash mm-chat/scripts/verify-agent-runtime-phase0.sh`.
- Broker gates: `bash mm-chat/scripts/verify-agent-broker.sh` and
  `bash mm-chat/scripts/verify-agent-broker-postgres17.sh`.
- Delegation gates: `bash mm-chat/scripts/verify-agent-delegation.sh` and
  `bash mm-chat/scripts/verify-agent-delegation-postgres17.sh`.
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
- G20.3 signatures are `internal/agentrunner`, `cmd/neo-runnerd`, migration
  `085`, `config/agent-runner`, `deploy/agent-runner`, and
  `scripts/verify-agent-runner*.sh`. It has no HTTP/startup import and does not
  enable Runtime. Exact-host readiness remains held at
  `ISOLATION_UNAVAILABLE` until an approved release passes as `neo-runner`.
- G20.4 signatures are `internal/agentbroker`, `internal/safenet`, migration
  `086`, strict Runner Prepare/Commit relay shapes and
  `scripts/verify-agent-broker{,-postgres17}.sh`. No package is imported by an
  HTTP/Chat/startup path; the default Runner relay returns
  `RUNTIME_UNAVAILABLE`, and production Project/object/vault/MCP mutation
  wiring remains held.
- G20.5 signatures are `internal/agentdelegation`, migration `087`, Runner
  `runLineage`, and `scripts/verify-agent-delegation{,-postgres17}.sh`. Root
  authority registration, Child enqueue/launch/settlement and cascade/recovery
  remain internal; no HTTP/Chat/startup import enables Child execution.
- PostgreSQL `agent_delegation_control` has SELECT plus exact function execution
  and no table DML. Root registration binds exact user/Project/Assistant,
  snapshot, model, package/runtime, Grant, Registry, expiry and budget. Child
  enqueue binds the exact live Parent Attempt generation/owner/token digest and
  atomically reserves wall/model-token/Tool-call/Artifact-byte budget.
- The authenticated control `UserID` is separate from every proposed Grant;
  proposed subject fields never select another user's Parent authority. Child
  subject/model/package/runtime are exact Parent bindings; Grant actions,
  resources, approval, Egress, Secrets, expiry and budgets may only narrow.
  Child Registry identities/capabilities must be a durable Parent subset after
  physical forbidden-set removal and before fingerprinting. Reused identities
  preserve capability, classification and idempotency class while actions,
  selectors, approval and call limits may only narrow; SQL prefix containment
  uses literal `starts_with`, never wildcard `LIKE`.
- Launch admission rechecks both leases, immutable fingerprints, exact Registry
  identities, expiry and current Parent/Child Kill Switches. Settlement requires
  an already terminal Child Run, is exact-replay idempotent and cannot exceed
  its reservation.
- Parent cancel/kill terminalizes every live Child Attempt/Step/Run and fences
  leases before invoking the reaper. Failed reap remains durable; reconcile
  discovers terminal, expired, reclaimed or Kill-Switched Parents and retries
  without restoring authority.
- PostgreSQL is the only intent/approval/receipt authority. Prepare binds
  subject, package/runtime/grant/registry, lease generation/owner/token digest,
  canonical arguments, approval, budget, expiry and Kill Switch epoch before
  any executor. Exact Commit replay returns the stored sanitized receipt;
  possible-send ambiguity becomes terminal `outcome_unknown`, never a blind
  retry.
- Cancellation and Commit serialize on the exact immutable intent. A valid
  cancellation may advance only `awaiting_approval|approved -> canceled` and
  `prepared Attempt -> canceled`; once Commit owns `committing`, cancellation
  cannot assert rollback. The cancellation fact is append-only and exact replay
  is stable.
- Grant revocation is append-only and checked by Prepare, Commit and Secret
  handle functions. Service-coordinated revocation also clears matching
  in-memory Secret bytes; terminal Commit, cancellation and expiry clear their
  corresponding memory-only handles after the PostgreSQL transition.
- `agent_effect_control` has SELECT plus exact function execution and no table
  DML. Public API, Orchestrator and Runner roles receive no effect authority;
  Runner remains credential-free.
- Agent Egress and MCP must share `internal/safenet`; Agent allowlists are exact
  HTTPS origins and reject IP literals. Redirect/dial resolution is rechecked,
  cross-origin credentials are stripped and environment proxies stay disabled.
- Secret plaintext and handle plaintext are memory-only. Durable state stores
  only handle digest, bindings, expiry and sanitized state. Handles are
  single-use, short-lived and non-renewable. Validate the exact committing
  intent, live lease and current Kill Switch before resolution and consume;
  terminal Commit revokes all remaining active handles.
- Until a production Project store exists, Project mutation is an interface plus
  deterministic CAS fake only. Artifact publication is object-before-row with
  compensating object deletion and never mutates Project. Recompute exact size
  and SHA-256 from one bounded quarantine snapshot, then scan and store those
  same bytes to close replacement/TOCTOU drift.
- PostgreSQL replay authority binds caller identity + Runner ID + request ID +
  nonce + authority-request fingerprint. The signed ticket carries the same
  nonce/fingerprint, and Runner ID must equal the current Attempt lease
  owner. The credential-free daemon independently fsync-claims the same request
  locally before any OCI action.
- Treat Podman flags as intent only. Compare exact post-create and post-start
  inspection for image, userns maps, UID/GID, caps, no-new-privileges, seccomp,
  namespace modes, cgroup manager/parent/CPU/memory/PID, log driver, bounded
  Scratch tmpfs and exact Workspace/Broker mounts.
- Workspace Resolve rewalks owner/modes/types/bounds and recomputes the full
  tree fingerprint before launch. Artifact intake is a per-Attempt framed Unix
  socket into quarantine; it never publishes or holds object-store credentials.
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
| Child subject/model/package/runtime/Grant/Registry widening | `SUBSET_VIOLATION`; no Child Run or reservation |
| concurrent Parent reservation exceeds any dimension | `BUDGET_EXCEEDED`; exact replay consumes no second reservation |
| stale Parent Attempt at enqueue/launch | `PARENT_STALE`; no Child launch |
| settlement before terminal Run or outcome drift | `SETTLEMENT_INVALID`; reservation remains held |
| reaper unavailable after cascade | Child lease remains fenced; reap becomes durable `failed` and reconcile retries |
| Draft attempts self-Promote | reject and security-audit |

### 5. Good / Base / Bad Cases

- **Good**: exact admitted package + frozen Grant creates a leased Attempt,
  non-root Runner starts a rootless Sandbox, Broker mediates I/O, PostgreSQL
  appends terminal facts and cleanup reaps all state.
- **Base**: all Runtime switches off; Chat/current text Skills continue and
  cleanup/reconciliation still runs after later persistence exists.
- **Bad**: browser assembles executable Skill context, model expands Tools,
  Backend runs code in-process, Runner uses rootful Docker, Child retains
  `delegate_task`, process-local Parent budgets accept concurrent Children,
  cascade re-enables a lease after reap failure, or a write reconnect is
  automatically retried.

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
- G20.3 source/control: `verify-agent-runner.sh`, disposable PostgreSQL 17
  `verify-agent-runner-postgres17.sh`, and expected-nonzero exact-host
  `verify-agent-runner-host.sh`. A source or fake-driver pass is never exact-host
  promotion evidence.
- Side effects: Prepare/Commit crash/acknowledgement-loss/idempotency matrix.
- G20.4 source/control: focused race tests for `internal/agentbroker`,
  `internal/safenet`, `internal/mcpclient` and `internal/agentrunner`; migration
  schema coverage; `verify-agent-broker.sh`; PostgreSQL 17
  `verify-agent-broker-postgres17.sh`; Runner/Phase 0 gates; and expected-nonzero
  exact-host `ISOLATION_UNAVAILABLE`. Advance every older PostgreSQL tail drill
  through migration `086` before accepting the slice.
- The G20.4 PostgreSQL drill must include Cancel-vs-Commit concurrency with one
  winner/zero dispatch when Cancel wins, Grant-revocation zero dispatch, and
  `outcome_unknown` atomic Run/Step/Attempt projection plus append-only events.
- G20.5 source/control: focused race tests for `internal/agentdelegation`,
  `internal/agentbroker`, `internal/agentrunner`, `internal/agentorchestrator`
  and migration schema; disposable PostgreSQL 17 fresh/replay, one-winner
  concurrent Parent reservation, least privilege, stale Parent/Child launch,
  terminal settlement, Child-first cascade, failed-reap recovery, automatic
  stale-Parent recovery, dump/restore and guarded down/up; then advance every
  older PostgreSQL tail drill through migration `087`.
- Child negatives: proposed Grant subject cannot replace authenticated user;
  forged/cross-user/non-root Parent, depth 2, widened subject/model/package/
  runtime/Grant/Egress/Secret/expiry/budget, same-identity Registry capability/
  action/resource/classification/idempotency rebind, SQL `_`/`%` prefix
  wildcard attempts, stale lease/fingerprint and Kill Switch all reject before
  launch.
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
