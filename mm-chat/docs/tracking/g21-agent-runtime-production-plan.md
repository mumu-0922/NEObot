# G21 Agent Runtime Production Activation Plan

## Baseline

G20.1-G20.10 and migrations `083`-`094` remain the authority. Legacy text
Skills are retired and admitted Package Skills are the only eligible Skill
domain. G21 advances production capability one independently evidenced stage
at a time; no later stage may be inferred from an earlier gate.

The development host remains `ISOLATION_UNAVAILABLE`. Exact-host artifacts,
credentials and live records stay outside Git. Every stage is default-off,
rollbackable by disabling its own worker/flag, and forbidden from widening a
frozen Grant or reviving browser/API/rootful execution.

## G21.0 — Exact-host bundle and control-plane maintenance

Status: source/control implementation complete; target-host activation held.

- Build and verify a deterministic content-addressed `neo-runnerd`/
  `neo-runner-probe` deployment bundle.
- Harden literal private/loopback bind and systemd probe/state boundaries.
- Require fresh `control_plane` activation evidence for exact release, policy,
  mTLS identities and five live checks.
- Run a separate least-privilege worker for `probe`, `list` and `reconcile`
  only. Keep Root Runs, Broker, Child, Cron and Learning false.
- Add default-off Compose/preflight wiring with a sixth PostgreSQL principal.

Promotion gate: run `scripts/verify-agent-runtime-g21-0.sh`, then reproduce a
real `ACTIVATION_READY` record on the approved target. Offline fixtures and the
current host cannot promote.

## G21.1 — Root Run launch canary

Status: source/control implementation complete; exact-host canary held.

- Add the independent `root_run_canary` activation stage, immutable canary
  plan, mTLS identity and Ed25519 authority key binding.
- Give Runner control and canary identities disjoint ingress method policies;
  control cannot launch and canary cannot call Broker Prepare/Commit.
- Run one idempotent synthetic depth-zero Run with an empty Tool Registry,
  read-only rootfs, `networkMode=none`, no Egress/Secret and bounded resources.
- Prove claim, signed launch, Runner/PostgreSQL heartbeats, signed cancel, exact
  reap and zero post-cancel Runner inventory.
- Atomically terminalize Sandbox, Attempt, Step and Run with three append-only
  cancellation events; roll back every projection on identity/state drift.
- Fence tokenless restart recovery until lease expiry and prefer exact signed
  cancel plus durable cancellation after a known-running heartbeat failure.
- Add a seventh LOGIN inheriting exactly `agent_orchestrator_runtime` and
  `agent_runner_control`, without adding migration `091` or direct table DML.
- Keep the dedicated Compose profile default-off and credential-free beyond
  its exact database, mTLS, activation, plan and signing-key inputs.

Promotion gate: run `scripts/verify-agent-runtime-g21-1.sh`, then reproduce a
fresh `ROOT_RUN_CANARY_GATES_PASSED` activation record and full canary flow on
the approved exact host. The disposable PostgreSQL proof and current-host
`ISOLATION_UNAVAILABLE` result cannot promote user-facing execution.

## G21.2 — Read-only Broker and Artifact canary

Status: source/control implementation complete; exact-host Broker/Artifact
canary held.

- Add a third independent `broker_artifact_canary` activation stage, caller,
  Compose profile and eighth PostgreSQL LOGIN.
- Relay Prepare/Commit from credential-free `neo-runnerd` over private literal
  HTTPS mTLS; independently verify the original authority ticket at the relay.
- Run exactly five synthetic one-Run actions: bounded Project/Workspace reads,
  one pinned auth-none MCP read, bounded Artifact publication and one
  acknowledgement-loss read.
- Require read/idempotent/automatic Registry authority; keep generic/mutable
  MCP, Project mutation, Egress, Secrets and Provider calls disabled.
- Add migration `091_agent_artifact_publication` with function-only
  `agent_artifact_control`, repeated live authorization and no table DML.
- Prove exact replay, stale lease/generation rejection, Artifact collision
  denial, same-byte scan, object-before-row compensation and quarantine cleanup.
- Terminalize an unresolved possible send as `outcome_unknown` and never issue
  a second Commit under any key.
- Keep Runner and Sandbox free of database, object-store, MCP, Provider, relay
  server-key and vault credentials.

Promotion gate: run `scripts/verify-agent-runtime-g21-2.sh`, then reproduce a
fresh `BROKER_ARTIFACT_CANARY_GATES_PASSED` record and the complete Runner-relay-
Broker flow on the approved exact host. Disposable PostgreSQL/object fakes and
the current `ISOLATION_UNAVAILABLE` host cannot promote mutable or user-facing
execution.

## G21.3 — Bounded mutable effects

Status: source/control implementation complete; exact-host Project mutation
canary held.

- Add a fourth independent `project_mutation_canary` stage, caller, private
  relay, Compose profile and ninth PostgreSQL LOGIN without changing earlier
  caller method sets.
- Admit exactly one synthetic `project.patch/project.write/apply_patch` CAS,
  one flat UTF-8 path and one exact resource; keep user Projects, deletes,
  arbitrary paths, MCP writes, generic Egress and Secrets disabled.
- Require a separate offline-signed Ed25519 `per_commit` approval. Mount only
  its public key and signed document; keep approval, Runner authority and TLS
  keys distinct.
- Use a domain-separated stable activation binding fingerprint so activation
  and approval document hashes do not form a cycle. Prepare first; a fixed
  approval ID plus durable collision fences prevents reuse for another intent.
- Add migration `092_agent_project_mutation_canary` with operator-provisioned
  synthetic resources, atomic CAS plus immutable receipt, exact status and
  replay-safe baseline cleanup under function-only authority.
- Recheck approval, live lease/generation, snapshot, Grant/Registry,
  revocation, Kill Switch, base revision, path, UTF-8 bytes and fingerprints at
  CAS. Stale, revoked or killed actions create zero receipt and no mutation.
- Resolve acknowledgement loss from the durable Project receipt. Clean
  unchanged base proves not sent; conflicting/unavailable status becomes
  terminal `outcome_unknown`; restart never dispatches a second CAS.
- Keep Runner and Sandbox free of Project database, approval, authority private
  key, relay server key, S3/MCP/Provider/vault and generic network material.

Promotion gate: run `scripts/verify-agent-runtime-g21-3.sh`, then reproduce a
fresh `PROJECT_MUTATION_CANARY_GATES_PASSED` record, exact offline approval,
complete Runner-relay-Broker-CAS-cleanup flow and zero residue on the approved
host. Disposable PostgreSQL and the current `ISOLATION_UNAVAILABLE` host cannot
promote user Projects or generic mutable execution.

## G21.4 — Depth-1 Child Agents

Status: source/control implementation complete; exact-host depth-one Child
canary held.

- Add a fifth independent `depth_one_child_canary` stage, lifecycle-only Runner
  caller, default-off Compose profile and tenth exact-membership PostgreSQL
  LOGIN. Preserve all earlier caller and relay method sets.
- Freeze one synthetic Parent and exactly one Child. Parent has one
  `delegate_task/create`; Child deliberately requests it while server
  derivation creates a capability-empty Grant and physically empty Registry.
- Preserve exact user/subject/model/Package/Runtime and require strict Child
  expiry plus four-dimensional budget subsets. Keep both Sandboxes rootless,
  read-only, capability-free, credential-free and `networkMode=none`.
- Recheck the exact Parent Attempt, generation, lease, snapshot, Grant,
  Registry, Kill Switch and reservation before enqueue/launch. Reject a second
  Child, depth two, stale attempts, aliases and every widening before Runner.
- Add migration `093_agent_child_canary_reap_transport` with function-only
  exact pending inventory, late-launch expiry fencing and replay-safe atomic
  Runner projection plus durable reap completion. Keep failures retryable and
  guard dirty down.
- Cascade and reap Child first, then settle budget and sign Parent cancel.
  Restart resolves existing lineage and failed reaps before work, waits lost
  tokens to expiry and never selects another Child idempotency key.
- Require terminal one-Parent/one-Child lineage, zero pending reaps and empty
  Runner inventory. Keep generic Runtime, Broker effects, Egress, Secrets,
  Scheduler, Skill install, Learning, API/Chat and public delegation disabled.

Promotion gate: run `scripts/verify-agent-runtime-g21-4.sh`, then reproduce a
fresh `DEPTH_ONE_CHILD_CANARY_GATES_PASSED` record and complete exact-host
Parent/Child launch, late-launch fencing, crash/restart and zero-residue proof.
Disposable PostgreSQL and the current `ISOLATION_UNAVAILABLE` host cannot
promote public delegation or depth two.

## G21.5 — Cron and Draft learning workers

Status: source/control implementation complete; exact-host Cron and Draft
activation held.

- Add independent `cron_worker` and `draft_learning_worker` activation stages,
  default-off Compose profiles and eleventh/twelfth PostgreSQL LOGINs. Require
  current G21.0-G21.4 evidence and migration head `095`; neither stage enables
  or reads the other.
- Add migration `094_agent_cron_learning_activation` with immutable operator-
  provisioned exact targets plus disjoint `agent_cron_worker` and
  `agent_learning_worker` NOLOGIN roles. Both are function-only and inherit no
  owner/control role or table DML.
- Bind Cron to one active synthetic Template revision/fingerprint and put the
  activation target inside every locking claim. Reuse migration-088 DST,
  missed, overlap, retry, approval, revocation, expiry and Kill-Switch semantics
  without giving Cron Runner, S3, Provider or administrator credentials.
- Bind Draft learning to one quarantined Draft, package/runtime/archive,
  pre-staged Workspace, Runner snapshot and fixed isolation/evaluation suites.
  Use Draft-only attempts that are not ordinary Agent Runs and cannot reach
  Broker, Provider, Cron, delegation or admission authority.
- Add the sixth Runner caller
  `spiffe://neo-chat/agent-runtime-draft-learning` with Probe/List/Reconcile/
  Launch/Result/Cancel only. Preserve all earlier caller policies and both
  relay routes; add no Heartbeat, Prepare or Commit.
- Add the read-only exact-Attempt `result` RPC. Validate a strict content-free
  result binding and persist its SHA-256 before canceling/reaping the exact
  Sandbox and completing the three-check migration-089 receipt bundle.
- Preserve human-only Promote/Reject and new immutable package-version
  creation. The worker has no review/admission credential; cleanup is exact-
  target and object-before-row after the separate human decision.
- Prove two due Templates/two quarantined Drafts, claim isolation, crash/replay,
  real LOGIN denials, dump/restore, guarded down and clean down/up. Advance every
  older PostgreSQL tail drill to final head `095`.

Promotion gate: run `scripts/verify-agent-runtime-g21-5.sh`, then reproduce
fresh exact-host READY evidence independently for the reviewed Cron and Draft
targets. Complete rootless isolation/evaluation, human decision, cleanup and
zero-residue proof on the approved host. Disposable PostgreSQL, deterministic
fakes and the current `ISOLATION_UNAVAILABLE` host cannot enable generic
Scheduler/Learning, autonomous promotion or a product cohort.

## G21.6 — Product canary and final closure

Status: source/control implementation complete; exact-host product activation
and final promotion held.

- Add migration `095_agent_product_canary_activation` with immutable bounded
  activation, append-only current-user requests, generation-fenced claims,
  terminal receipts and final-promotion facts.
- Give `go_api_runtime` only status/enqueue functions. The strict request body
  carries the expected Shadow policy revision and opt-in generation; the server
  owns request ID/fingerprint and accepts no execution payload or override.
- Add the thirteenth LOGIN with exactly `agent_product_canary_worker`,
  `agent_orchestrator_runtime` and `agent_runner_control`. Keep the worker role
  function-only and API/worker/operator authority disjoint.
- Add the seventh caller `spiffe://neo-chat/agent-runtime-product-canary` with
  Probe/List/Reconcile/Launch/Heartbeat/Cancel only. Preserve every earlier
  caller and relay policy.
- Freeze one private bounded-smoke plan: fixed image/argv, UID/GID `10001`,
  depth zero, empty Tool Registry, read-only rootfs, `networkMode=none`, no
  Egress/Secrets and strict CPU/memory/PID/wall/output/scratch limits.
- Require a fresh production `product_canary` activation binding migration head
  `095`, the exact release/policy/Package/Runtime/plan and seven distinct
  G21.0-G21.5 evidence fingerprints. Activation expiry, disablement, drift,
  budget or Kill Switch blocks new requests immediately.
- Bind completion to one exact request, Run, Attempt, snapshot, plan and
  content-free receipt. PostgreSQL independently derives request/receipt
  fingerprints; concurrent enqueue converges on one request, and expired claims
  pass through bounded reconcile before reclaim. Crash/restart reuses the same
  idempotency key and waits lost lease/authority expiry before reconciliation.
- Keep health read-only: query stale claims and pending terminalizations, then
  require Runner Probe/List to report zero Sandbox residue.
- Keep final promotion separate: only the operator may append a
  `PROMOTION_READY` fact binding the exact activation, receipt, release and
  closure fingerprint after the read-only evaluator passes.
- Extend closure with the full activation chain, exact product receipt,
  product queue/stale/failure/budget metrics and zero temporary product
  request/claim/receipt residue while retaining sanitized promotion/audit facts.
- Prove enabled preflight, PostgreSQL ACL/lease/replay/dump/restore/down-up,
  canonical/concurrent enqueue, read-only health, activation/closure, Compose,
  frontend, Phase 0, G21.0-G21.5 regression and full standalone. Every older
  tail drill peels the empty `095` tail before its earlier guard and returns to
  head `095`.

Promotion gate: run `scripts/verify-agent-runtime-g21-6.sh`, then reproduce the
exact-host clean-copy/restart/reboot, paired backup/restore, DR,
rollback/forward-fix, rotation, alerts, capacity and cleanup matrix. The exact
release must evaluate to `PROMOTION_READY`. Offline fixtures, disposable
PostgreSQL and this development host cannot create activation or promotion
authority.

## Epic completion

- Every enabled package Run uses the exact accepted rootless Runner path.
- Every action is Grant/lease/idempotency/Kill-Switch fenced.
- Child depth is at most one; Cron and Learning cannot create authority.
- No legacy, browser, API in-process, rootful Docker or public Runner fallback
  exists.
- Backup/restore, clean restart, rollback and current evidence reproduce from a
  clean baseline.
