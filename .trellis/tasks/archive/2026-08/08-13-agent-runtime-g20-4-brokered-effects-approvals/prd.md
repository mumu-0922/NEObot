# Agent Runtime G20.4 — Brokered Tools, Egress, Secrets and Side Effects

## Goal

Implement the held Agent Broker control foundation: deterministic Tool Registry
construction from frozen Capability Grants, mediated Egress and Secret use,
PostgreSQL-durable Prepare/approval/Commit authority, replay-safe executor
receipts, bounded Project patch CAS and Artifact publication seams. The work
must preserve the credential-free non-root Runner boundary and must not enable a
public Agent Runtime, Chat integration or mutable production effect.

## Shared baseline

- G20.1 supply-chain, G20.2 durable Orchestrator and G20.3 rootless Runner
  source/control foundations remain binding.
- PostgreSQL 17 is the only durable Run/Attempt/approval/intent/receipt/Kill
  Switch authority. Redis is never effect authority.
- `neo-runnerd` remains non-root and receives no PostgreSQL, Redis, object-store,
  Provider, vault, browser bearer, MCP credential or runtime socket credential.
- The current host remains `ISOLATION_UNAVAILABLE`; source/fake Broker tests are
  not exact-host promotion evidence.
- Current Agent API/Chat/frontend routes remain absent. `/v1/code/executions`
  remains fail closed. Current pure-text Skills remain unchanged through G20.8
  and are hard-deleted only in G20.9.
- All recommended design choices are authorized without another confirmation.
  No sub-Agent delegation is used.

## Requirements

### Deterministic Tool Registry and authorization

- Add an isolated `internal/agentbroker` package. Build a canonical Registry by
  intersecting current server-owned Tool definitions, the admitted package
  declarations and exact frozen `neo.capability-grant/v1` capabilities.
- Bind canonical Tool identity, capability, actions/resources, approval class,
  per-capability calls, total Tool calls, Egress, Secret refs, package/runtime,
  snapshot, subject, Run lineage, expiry and registry fingerprint.
- Unknown Tool/action/resource, alias confusion, duplicate definition,
  unsorted/colliding selector, exhausted budget, expired/revoked Grant, stale
  lease, snapshot drift and applicable Kill Switch all fail before an executor.
- For depth 1, physically remove `delegate_task`, `cron_manage`, `grant_manage`,
  `secret_manage` and `runtime_manage` before fingerprinting and reject any
  forged Runner registry that contains them.

### Durable Prepare, approval and Commit

- Add migration `086` for immutable effect intents, append-only approval
  decisions, Commit claims/receipts, handle-digest state and retention indexes.
- Bind each intent to user/Project/Assistant, Run/Step/Attempt/generation,
  current lease owner/token digest, snapshot/grant/registry fingerprints,
  capability/action/resource, canonical arguments fingerprint, approval class,
  base resource revision, expiry, Kill Switch epoch and one stable idempotency
  key.
- Prepare performs strict authorization and canonical validation but no mutable
  external/Project effect. Exact Prepare replay returns the same intent;
  mismatch is `REPLAY_DETECTED`.
- `automatic` records a server approval fact; `once` and `per_commit` require an
  authenticated human decision bound to the exact intent fingerprint. The
  Sandbox/model/Tool cannot approve an effect or reuse an approval for another
  intent. Deny and expiry are terminal.
- Commit atomically rechecks the live lease generation/token digest, frozen
  fingerprints, approval, budget, revocation, Kill Switch and intent expiry,
  then claims the stable key before crossing the executor boundary.
- Exact terminal replay returns the original sanitized receipt. A key reused
  with a different intent is rejected and audited. Cancellation before accepted
  Commit wins; after external send it cannot claim rollback.
- Executor acknowledgement loss performs an exact status/receipt query. If the
  executor cannot prove committed or not-sent, terminalize the Attempt/Step/Run
  as `outcome_unknown`; never automatically Commit again.

### PostgreSQL authority and least privilege

- Use `agent_effect_owner` and narrow `agent_effect_control` roles. The future
  Backend control worker may assume the control role; `go_api_runtime`,
  `agent_orchestrator_runtime`, `agent_runner_control` and `neo-runnerd` gain no
  direct table DML or broad ownership.
- All mutation is through exact SECURITY DEFINER functions with a pinned
  search path. Runtime roles receive only required SELECT/function EXECUTE.
- Runtime disabled and active Kill Switches still permit expiration, receipt
  reconciliation, retention and cleanup. Down migration refuses while any
  effect/approval/receipt/handle authority exists.

### Egress Broker

- Support `none`, `allowlist` and `brokered` modes. `none` performs zero network
  action. `allowlist` accepts exact HTTPS/WSS host+port rules only. `brokered`
  lets a registered Tool executor construct requests; Sandbox input never
  chooses arbitrary credentials or headers.
- Refactor/reuse the current MCP safe-network primitive rather than duplicate
  DNS/IP policy. Reject raw/alternate IP literals, userinfo, fragments,
  localhost, private/link-local/multicast/metadata/documentation/benchmark
  ranges and unapproved scheme/host/port.
- Resolve and validate every dial; redirects and reconnects repeat policy.
  Strip credentials on any origin change. Bound method/header/request/response
  bytes, redirects, wall time and concurrency. No proxy-from-environment.
- Durable diagnostics contain only rule/destination fingerprints and bounded
  counts/error classes, never URL paths/query, headers, request/response bodies
  or resolved private coordinates.

### Secret Broker

- Grant contains opaque `secret_ref` values only. Resolve vault bytes for the
  exact subject/Attempt/generation/capability/action/destination/intent and
  create a single-action, non-renewable, short-TTL opaque handle.
- Persist only handle digest, binding fingerprints, expiry and sanitized state.
  Never persist plaintext handle or secret. Prefer Broker-side credential
  application so neither value reaches Sandbox.
- Revoke handles on use, terminal, kill, lease expiry/reclaim, intent expiry,
  Secret/Kill Switch revocation and cleanup. Wrong binding, replay and renewal
  fail closed.
- Synthetic canaries prove no secret/handle enters prompt, env, argv, `/proc`,
  Workspace, retained Scratch, Artifact, Tool result/error, PostgreSQL, event,
  log, metric or diagnostic report.

### Tool/MCP executor boundary

- Define narrow read-only and mutable executor interfaces. Read-only actions may
  execute directly only when their policy is explicitly idempotent and all
  current fences pass. Mutable/unknown actions require a committed intent.
- Reuse the existing MCP connector/schema/result controls through an adapter;
  do not treat current Conversation MCP call rows as Agent intent authority.
- Never retry a mutable MCP/external call after possible dispatch. Persist only
  a bounded sanitized result/receipt fingerprint and executor status token
  digest; raw arguments/results/provider payloads stay outside events/logs.

### Project patch and Artifact seams

- Add a canonical bounded Project patch intent with exact base revision,
  path/type/size limits and content fingerprint. Reject absolute/traversal/NUL,
  duplicate Unicode/case collisions, links/special files and unbounded bytes.
- Project Commit calls a CAS executor with exact base revision and stable key;
  revision conflict is terminal for that intent and never silently rebases.
  Because no production Project-file store exists, only the interface and
  deterministic CAS acceptance fake are implemented; production mutation stays
  held.
- Authorize Runner-quarantined Artifact publication only after current
  Attempt/grant/quota and type/secret/malware policy. Use object-before-row plus
  compensating object deletion. Artifact publication never mutates Project.

### Runner/control seam and recovery

- Activate strict `prepare`/`commit` shapes already reserved in
  `neo.runner-rpc/v1`, adding exact authority-ticket and fingerprint bindings
  needed to prevent stale/moved requests. Unknown/duplicate/malformed fields,
  method mismatch and replay fail before Broker action.
- Runner remains a credential-free relay/local channel, not durable approval or
  effect authority. It delegates to an injected control-plane Broker interface;
  production outbound wiring remains disabled until the private authenticated
  channel and exact-host suite are available.
- Runner and Backend restart recover from PostgreSQL intent/receipt state plus
  Runner replay fences. Reconciliation distinguishes not-sent, exact receipt
  and unknowable outcome without inventing rollback.

### Diagnostics, verification and held promotion

- Add focused race/vet/unit suites, strict schema fixtures, migration schema
  coverage, a disposable PostgreSQL 17 concurrency/least-privilege/crash/
  retention/dump-restore/down-up drill, and an offline G20.4 verifier.
- Add synthetic SSRF/DNS rebinding/redirect/reconnect tests, Secret zero-leak,
  approval replay, Project CAS, Artifact quarantine and complete Commit crash
  matrix. Fakes prove control behavior only and self-identify as non-production.
- Advance every legacy PostgreSQL tail drill through migration `086` without
  weakening its owning earlier guard.
- Update architecture/contracts/deployment/tracking/Trellis specs. No live host,
  account, package, certificate, secret, Compose, API, Chat or frontend change.
- G20.4 completion means source/control foundation complete with production
  promotion held. Only an official synthetic read-only canary is eligible
  before mutable promotion, and it is not run by this task.

## Acceptance criteria

- [x] Registry construction is deterministic and rejects alias/selector/grant/
      snapshot/expiry/budget/Kill-Switch drift before execution; depth-1
      forbidden Tools are physically absent.
- [x] Prepare is mutation-free, replay-stable and exact-intent bound; approvals
      cannot be forged, moved, reused across intent or granted by Sandbox/model.
- [x] Commit concurrency yields one executor dispatch and one receipt; exact
      replay returns it, mismatch rejects, and acknowledgement loss becomes
      `outcome_unknown` when no exact status proof exists.
- [x] Stale lease/generation/token, expired/revoked Grant/intent, active Kill
      Switch or budget exhaustion performs zero executor/network/Secret action.
- [x] Egress tests reject alternate IP, private/metadata, DNS rebinding,
      redirect/reconnect drift and cross-origin credential forwarding while
      enforcing exact allowlist and byte/time/concurrency caps.
- [x] Secret canary and handle replay/expiry/wrong-binding tests prove zero
      plaintext persistence/return and exact revocation.
- [x] Mutable MCP adapter never retries after possible dispatch; read-only retry
      requires explicit idempotent policy and no observed result.
- [x] Project patch malicious paths/collisions/size cases fail atomically; exact
      CAS Commit succeeds once and base-revision conflict never rebases.
- [x] Artifact publication rejects stale authority, quota/type/secret/malware
      failures and cleans partial object/quarantine state.
- [x] Migration `086` passes PostgreSQL 17 fresh/replay/concurrency/least
      privilege/crash-reconcile/retention/non-empty-down/dump-restore/clean
      down-up; Runner has no database credential or role.
- [x] Focused race/vet/tests, Backend, Frontend, RAG, Phase 0 and full standalone
      clean-copy gates pass; protected runtime paths remain untouched.
- [x] Exact-host acceptance remains nonzero `ISOLATION_UNAVAILABLE`; no
      source/fake evidence promotes production Runtime or mutable effects.
- [x] Agent HTTP/Chat/frontend and legacy text Skills remain unavailable and
      unchanged.

## Deliverables

- `mm-chat/backend/internal/agentbroker/` with README/DESIGN and tests.
- `mm-chat/backend/migrations/086_agent_broker_foundation.{up,down}.sql` and
  schema/PostgreSQL integration coverage.
- Strict Runner Prepare/Commit relay/fence changes and tests.
- Shared safe-network policy extraction with MCP regressions.
- `mm-chat/scripts/verify-agent-broker*.sh` and advanced migration drills.
- Agent Runtime schemas, fixtures, docs, tracking and Trellis specs synchronized
  to the held G20.4 foundation.

## Out of scope

- Public Agent Run/approval HTTP routes, frontend UI, Chat process integration,
  startup workers or production Agent execution.
- Real Project-file store integration, live MCP/provider writes, live Secret
  vault values, production Artifact attachment or mutable canary.
- Host provisioning, exact rootless stack installation, live certificates,
  service restart, Compose wiring or production release approval.
- Child Agent depth 1 implementation (G20.5), Cron (G20.6), Draft learning
  (G20.7), product/shadow execution (G20.8), legacy Skill deletion (G20.9).

## Technical approach

Use a Backend-owned `agentbroker` bounded context and migration `086` as the
single durable effect coordinator. Keep executors behind narrow interfaces and
make all authority/fingerprint/idempotency decisions before calling them.
Refactor the existing MCP safe HTTP policy into a shared internal package to
avoid security drift. Extend Runner Prepare/Commit as an authenticated,
replay-fenced relay only; it gains no credential or database authority.

## Decision (ADR-lite)

**Context:** Direct Sandbox network/secret access or direct reuse of Chat MCP
writes would bypass frozen Agent grants, stale-lease fencing and durable effect
recovery. Letting Runner own approvals would violate its credential-free trust
boundary.

**Decision:** Centralize Registry, Prepare/approval/Commit and receipts in a
PostgreSQL-backed Backend Broker Coordinator; mediate Egress/Secrets/executors
through narrow interfaces; keep Runner a strict credential-free relay; leave
all user-facing and production wiring held.

**Consequences:** G20.5+ can consume one auditable effect seam and ambiguous
writes become explicit terminal facts. This group is larger and adds migration
and adapter work, but avoids dual authority. Production Project mutation and
exact-host promotion remain separate release gates.

## Research references

- [`research/durable-effects-and-approvals.md`](research/durable-effects-and-approvals.md)
- [`research/egress-and-secret-brokers.md`](research/egress-and-secret-brokers.md)
- [`research/reuse-project-artifact-and-runner-seams.md`](research/reuse-project-artifact-and-runner-seams.md)
