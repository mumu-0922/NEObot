# Agent Runtime Backend Contract

## Scenario: Durable package-Skill execution

### 1. Scope / Trigger

Apply this contract for Agent Skill admission, Run/Step/Attempt persistence,
Capability Grants, Runner RPC, side effects, Child Agents, Cron, Draft learning,
Kill Switches, or the legacy text-Skill cutover. G20.1 implements the no-execute
supply chain, G20.2 the internal durable Orchestrator, G20.3 the held Runner,
G20.4 the held Broker, G20.5 the held depth-1 delegation and G20.6 the held
durable Cron scheduling foundation, G20.7 Draft learning, G20.8 the held
Agent Center/Shadow product facade, and G20.9 the legacy text-Skill hard
retirement. G20.10 adds the held production-policy/evidence gate without a new
migration or worker. MCP and `/v1/code/executions` execution behavior remains
unchanged.

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
- Product/Shadow gates: `bash mm-chat/scripts/verify-agent-product-shadow.sh`
  and `bash mm-chat/scripts/verify-agent-product-shadow-postgres17.sh`.
- Production closure gate:
  `bash mm-chat/scripts/verify-agent-production-closure.sh`.
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
- G20.9 hard-deletes legacy text-Skill definitions/selections/execution
  paths without migration/wrapping. Historical messages retain only the
  read-only fact “旧版技能已退役”. Package execution remains held when exact-host
  isolation is unavailable.
- G20.10 policy and closure JSON are strict, content-free and exact-release
  bound. The read-only evaluator derives ready/held/invalid and never activates
  Runtime. The checked-in template remains `ISOLATION_UNAVAILABLE`.
- G20.8 intentionally imports `internal/agentlearning` only from the
  authenticated `agentcontrol` facade and `cmd/api` construction. Startup must
  pass `WithLearningEnabled(false)` and must not call learning Claim,
  Reconcile, Cleanup or Prune. Source gates must allowlist those exact held
  importers rather than preserving the obsolete G20.7 blanket no-import check.

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
| closure record is template/stale/incomplete/drifted or has residue | held/invalid; no activation |

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
- G20.10: strict policy/closure schemas, ephemeral positive semantics, held,
  stale, policy drift, incomplete/duplicate checks, cleanup residue and unsafe
  file inputs; exact-host remains separately expected-nonzero here.

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

## Scenario: Durable Cron scheduling foundation

### 1. Scope / Trigger

Apply this contract when creating or changing an Agent Cron template revision,
schedule calculator, durable cursor/claim/occurrence flow, trigger admission,
retry/overlap policy, lifecycle, reconciliation or retention. G20.6 is a held
control-plane foundation: it may atomically create a normal Orchestrator Run,
but it does not expose a public API or enable a startup/production Scheduler.

### 2. Signatures

- Go package: `mm-chat/backend/internal/agentcron`.
- Database authority: migration `088_agent_cron_foundation` and role
  `agent_cron_control`.
- Contract schema: `neo-cron-template.schema.json` with its valid and invalid
  fixtures.
- Source gate: `bash mm-chat/scripts/verify-agent-cron.sh`.
- PostgreSQL 17 gate:
  `bash mm-chat/scripts/verify-agent-cron-postgres17.sh`.

### 3. Contracts

- A logical template stores lifecycle, current revision, exact next UTC cursor
  and claim state. Changing any frozen owner/input/schedule/timezone/model/
  budget/Skill/Grant/Registry/Egress/Secret/expiry/step/scope/policy field creates
  a new immutable revision and append-only automation approval.
- Accept exactly a standard five-field Cron expression. Reject seconds,
  descriptors, `TZ=`/`CRON_TZ=` prefixes and unknown fields. Load the separately
  frozen IANA timezone from embedded Go tzdata; the calculator identity is
  `robfig-cron/v3.0.1+go-tzdata`.
- Spring-forward gaps create no occurrence; fall-back overlaps create both UTC
  instants. Durable occurrence identity is
  `template + revision + scheduled UTC instant`.
- PostgreSQL migration `088` is the only durable template, revision, approval,
  cursor, claim, occurrence, audit and Run-link authority. Candidate selection
  may use `FOR UPDATE SKIP LOCKED`, but every cursor advance, enqueue, release
  and terminal transition must recheck owner + generation + expiry under the
  exact row lock.
- Missed policy is only `skip`, `fire_once` or bounded `catch_up` with a maximum
  24-hour window and `maxCatchupRuns <= 100`. Overlap is only `skip`,
  `buffer_one` or `allow`; retry is bounded and reuses the same occurrence and
  Run idempotency identity. Never retry a proven enqueue or an external effect.
- Enqueue atomically links the occurrence to a normal Orchestrator Run. Trigger
  admission rechecks current revision, live owner, exact installed/admitted
  package/runtime, unrevoked Grant and approval, expiry, relevant Kill Switches,
  budget and overlap. It never falls forward to a latest revision, Skill,
  Grant, model or Secret.
- `agent_cron_control` has SELECT plus exact `SECURITY DEFINER` function
  execution and no table DML. Runner receives no database, vault, object-store,
  Provider or Cron credentials.
- Pause stops future materialization; resume advances to the next future instant
  without surprise backfill; delete tombstones. Runtime/Scheduler disabled still
  permits reconciliation, exhausted-retry terminalization, audit reads and
  bounded cleanup.
- No HTTP, Chat, frontend, startup, Redis wake, Compose or production
  Runner/Broker path may import or invoke G20.6. Exact-host promotion remains
  `ISOLATION_UNAVAILABLE`, and legacy text Skills remain until G20.9.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| invalid/ambiguous schedule, timezone, policy, budget or content-bearing metadata | reject revision; persist nothing |
| stale cursor/trigger generation, owner or expired claim | `STALE_CLAIM`; no advance, enqueue, release or terminalization |
| claimed revision is no longer the active exact revision | `STALE_TEMPLATE`; sanitized skipped fact, no Run |
| owner deleted | `OWNER_REVOKED`; sanitized skipped fact, no Run |
| Skill uninstalled, rejected or runtime fingerprint drifted | `SKILL_REVOKED`; no fallback and no Run |
| Grant or automation approval revoked | `GRANT_REVOKED` / `APPROVAL_REVOKED`; no Run |
| revision expired | `TEMPLATE_EXPIRED`; no Run |
| matching Secret Kill Switch active | `SECRET_REVOKED`; no Run or Secret resolution |
| global/scheduler/user/project/skill Kill Switch active | `KILL_SWITCH_ACTIVE`; no Run |
| live overlapping Run under `skip` / full `buffer_one` | `OVERLAP_SKIPPED` / `OVERLAP_BUFFER_FULL`; no replacement Run |

### 5. Good / Base / Bad Cases

- **Good**: two workers race on one due cursor; one fenced generation records a
  unique UTC occurrence and one stable-idempotency Orchestrator Run, while replay
  after acknowledgement loss returns that same Run.
- **Base**: Runtime and Scheduler remain disabled; no trigger loop starts, while
  an authorized control-plane operator can reconcile expired claims and prune
  eligible terminal history in bounded batches.
- **Bad**: a process-local timer advances an in-memory cursor, accepts embedded
  timezone syntax, unboundedly backfills after an outage, or enqueues with the
  latest Skill/Grant after its claimed revision lost authority.

### 6. Tests Required

- Unit/race tests: strict parsing and unknown fields, fingerprint stability, IANA
  timezone validation, DST gap/overlap, bounded missed policies, lifecycle,
  stale claims, retry, overlap and occurrence idempotency.
- Migration schema tests must pin immutable tables/functions, narrow grants,
  sanitized rows and guarded rollback.
- The PostgreSQL 17 drill must cover fresh/replay, concurrent claims,
  crash/restart reclaim, stale advance/release/enqueue, acknowledgement-loss
  replay, bounded retry terminalization, all overlap modes, owner/Skill/Grant/
  approval/expiry/Kill-Switch denials, dump/restore, least privilege, guarded
  down and clean down/up.
- Every older PostgreSQL drill that peels tail migrations must down empty `091`,
  `090`, `089` and then `088` before testing its own guard, then reapply through
  head `091`.
- Phase 0 and source gates must prove the package has no public/startup wiring;
  the exact-host gate must remain expected-nonzero `ISOLATION_UNAVAILABLE`.

### 7. Wrong vs Correct

#### Wrong

```text
process-local timer -> mutable template -> enqueue with latest authority
```

#### Correct

```text
strict Go Cron + embedded tzdata -> immutable approved revision
-> PostgreSQL-fenced UTC cursor/occurrence -> trigger-time authority recheck
-> atomic stable-idempotency Orchestrator Run link
```

## Scenario: Immutable Draft-only Agent learning

### 1. Scope / Trigger

Apply this contract when changing Agent learning proposals, Draft archives,
provenance/tests, check adapters or claims, administrator review/Promote,
learning admission, quarantine cleanup, migration `089`, or the Skill supply
`learning` source. G20.7 is a held control-plane foundation with no public API,
startup worker, Chat/frontend/Compose wiring or production Draft execution.

### 2. Signatures

- Go package: `mm-chat/backend/internal/agentlearning`.
- Database authority: migration `089_agent_draft_learning`, owner
  `agent_learning_owner`, and control role `agent_learning_control`.
- Contract schema: `neo-skill-draft.schema.json` with valid/invalid fixtures.
- Source gate: `bash mm-chat/scripts/verify-agent-learning.sh`.
- PostgreSQL 17 gate:
  `bash mm-chat/scripts/verify-agent-learning-postgres17.sh`.

### 3. Contracts

- Accept only a same-user terminal `succeeded` depth-0 Run whose immutable
  snapshot binds the exact existing base package. Child, queued/failed,
  cross-user, stale-snapshot and unknown-base proposals persist no Draft.
- Revalidate both base and proposal through `skillsupply.ValidateArchive`.
  Require `SKILL.md`, an exact Neo Runtime Manifest and at least one test under
  `tests/`. Diff and Promote must bind the revalidated base archive back to the
  exact stored base package fingerprint, not merely compare equivalent runtime
  authority. Proposed package/version must differ and the package must not
  exist.
- Learning may change only the package version inside the authority envelope.
  Runtime image/platform/user, entrypoints, dependencies, `allowed-tools`,
  capability/Egress/Secret requests, resources and limits cannot widen or
  rebind.
- The immutable Draft binds source Run/snapshot, base/proposed package,
  runtime/SBOM/archive, exact test inventory, changed paths and bounded
  source-package/Run-event evidence. Every changed path must be mapped to Run
  evidence. Prompt/input/output, Tool, Secret/credential and Workspace bodies
  are forbidden from durable Draft/check/audit documents.
- Draft bytes use only `skill-drafts/sha256/<archive>.zip`. Existing bytes at a
  content-addressed key must match exactly; mismatch is `DRAFT_OBJECT_DRIFT`
  and is never overwritten.
- PostgreSQL claims checks by owner/generation/expiry. Exactly one `static`,
  `isolation` and `evaluation` result for the live generation is required.
  Static high-confidence prompt override, secret copy, source laundering or
  evaluation gaming fails policy. Policy failure is terminal for the Draft;
  infrastructure retry is bounded and stale workers cannot publish/release.
  Claim functions select only ready `quarantined`/`pending` work; expired
  `checking`/cleanup claims must first pass through reconciliation so the
  failed attempt is counted and can terminalize before a new generation.
- Only the configured authenticated administrator may Reject or Promote using
  exact revision, Draft/package fingerprints and reason. Exact replay is
  stable and reads the append-only decision even after delayed quarantine
  cleanup removed Draft bytes. Promote rehashes and revalidates bytes/authority,
  verifies current source/Kill Switches and exact receipts, writes canonical
  source/package/SBOM objects, then atomically creates one new immutable
  admitted `learning` candidate/package. It never edits a package,
  installation, Run/snapshot, Grant, Secret, Runtime or Cron revision.
- Rejected/promoted quarantine cleanup is owner/generation/expiry fenced and
  object-before-row. Learning disabled still permits read/audit, reconcile,
  cleanup and bounded prune. Default isolation/evaluation adapters remain
  unavailable and never execute a package in-process.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| source Run is cross-user, non-succeeded, depth 1 or snapshot/base drifted | `SOURCE_RUN_INVALID`; no Draft row |
| archive, Manifest, tests, evidence or path binding is invalid | reject before persistence; no admission |
| proposal changes runtime/Tool/capability/Egress/Secret/resource authority | `AUTHORITY_WIDENED`; no Draft row |
| proposed package already exists | `PACKAGE_ALREADY_EXISTS`; no Draft row |
| quarantine/canonical object key contains mismatched bytes | `DRAFT_OBJECT_DRIFT`; do not overwrite or promote |
| base object validates but no longer matches the stored base package fingerprint | `DRAFT_OBJECT_DRIFT`; no diff or promotion |
| check owner/generation/expiry is stale | `STALE_CLAIM`; no result/release/cleanup acknowledgement |
| a check fails policy or exact three receipts are absent | `check_failed` / `CHECKS_INCOMPLETE`; no in-place wash or candidate |
| non-administrator, stale revision, mismatched fingerprints or active Kill Switch promotes | administrator/revision/promotion/Kill denial; no live mutation |
| cleanup deletion fails | retain retryable cleanup row; never acknowledge deletion first |

### 5. Good / Base / Bad Cases

- **Good**: a succeeded root Run proposes a version-only package, exact three
  fenced checks pass, the administrator reviews an ephemeral diff and Promote
  creates one new admitted fingerprint while the base/install/Run/Cron remain
  byte-authoritative.
- **Base**: Learning and Runtime remain disabled; no worker or public surface
  runs, while reconciliation and object-before-row cleanup remain callable.
- **Bad**: model output edits the installed Skill, expands a Grant/runtime,
  retries a failed evaluation against the same Draft, auto-promotes on score,
  overwrites a content-addressed collision, or deletes a cleanup row before the
  object.

### 6. Tests Required

- Unit/race tests cover archive/Manifest/test/provenance validation, authority
  equality, canonical fingerprints, diff limits, prompt/secret/laundering/
  gaming attacks, default-off adapters, collision drift and cleanup ordering.
- Migration schema tests pin the five tables, exact functions, immutable facts,
  sanitized fields, narrow grants, internal `learning` source and guarded down.
- PostgreSQL 17 proves fresh/replay, least privilege, source authority, exact
  three-check claims, stale generations, human-only Reject/Promote replay,
  replay after quarantine cleanup, bounded reconcile-before-reclaim, immutable
  base/live state, cleanup/restart, dump/restore, guarded down and clean
  `088 -> 089 -> 088 -> 089`.
- Exercise failed object read/delete release paths, not only successful cleanup.
  PL/pgSQL retry locals must use unambiguous names such as `next_attempts`
  rather than shadowing an `attempts` column.
- Every older PostgreSQL tail drill peels empty `091`, then its reviewed tail,
  before its original guard and finishes at head `091`. Phase 0 validates the strict Draft schema,
  G20.8 product/Shadow signatures and cross-contract bindings.
- Full standalone must pass and exact-host Runner verification remains expected
  nonzero `ISOLATION_UNAVAILABLE`.

### 7. Wrong vs Correct

#### Wrong

```text
successful Run -> evaluator score -> overwrite installed Skill -> next Run
```

#### Correct

```text
succeeded depth-0 Run + exact base -> immutable quarantined Draft
-> reconcile-before-reclaim -> generation-fenced static/isolation/evaluation
-> human Promote -> new admitted immutable package
-> delayed object-before-row cleanup -> decision-only exact replay
```

## Scenario: Authenticated Agent Center and held Shadow

### 1. Scope / Trigger

Apply this contract when changing `internal/agentcontrol`, `/v1/agent-center/*`,
Agent Center projections/mutations, Artifact downloads, migration `090`, Shadow
policy/opt-in/observations or the G20.9 retirement boundary. G20.8 exposes
control and review only; it does not enable package execution.

### 2. Signatures

- Backend: `mm-chat/backend/internal/agentcontrol/`.
- Database: `090_agent_product_shadow` and `agent_product_owner`.
- Frontend: `components/agent/AgentCenter.tsx` and the typed `agentCenterApi`.
- Gates: `verify-agent-product-shadow{,-postgres17}.sh`,
  `verify-agent-legacy-cutover{,-postgres17}.sh` and Phase 0.

### 3. Contracts

- Bind every non-admin read/mutation to the authenticated user. Draft reads and
  review plus Shadow policy update require the configured administrator.
- Keep package Skills, Assistants and MCP as separate identity/authority
  domains. Package Store/library continues through `/v1/skills/*`; legacy
  text-Skill editor/executor authority no longer exists after G20.9.
- Handlers compose owning services. They never issue table DML, lease/claim
  worker work, Commit effects, enqueue Cron triggers or run learning checks.
- Cancellation binds exact state/snapshot fingerprint; approval, Cron and Draft
  mutations keep their existing request/revision/fingerprint replay fences.
- Artifact list DTOs omit object keys. Download resolves the exact
  user/Run/Artifact tuple server-side and streams only through injected private
  object storage.
- Shadow defaults off, supports only `synthetic|read_only`, requires admin
  policy plus user opt-in and binds cohort/revision/generation/boot/admission/
  package/runtime. Effective capabilities exclude writes, Secret, Egress,
  delegation, Cron creation and Draft promotion.
- Global/user/admission/Skill Kill Switch, opt-out, expiry, restart, budget and
  fingerprint drift fence observations. Persist only IDs/fingerprints,
  mode/outcome/reason, latency bucket and bounded counts.
- Missing exact-host isolation returns `ISOLATION_UNAVAILABLE`; never execute
  package code in API/browser or inject Shadow output into Chat/admission/
  promotion.
- G20.9 removes legacy browser execution. Chat Conversation create/update/read
  strips `activeSkills`, and Package execution remains held rather than falling
  back to browser/API execution.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| cross-user Run/Artifact/cancel | `NOT_FOUND`; no object lookup or mutation |
| non-admin Draft/Shadow policy access | `ADMINISTRATOR_REQUIRED` |
| stale expected revision/fingerprint/generation | stable conflict; no authority change |
| invalid server DTO | frontend `INVALID_SERVER_RESPONSE` |
| Shadow disabled/opt-out/cohort miss | stable held reason; no adapter call |
| applicable Kill Switch | SQL/service `KILL_SWITCH_ACTIVE`, sanitized HTTP `AGENT_AUTHORITY_DENIED`; no adapter/observation |
| exact host unavailable | `ISOLATION_UNAVAILABLE`; no executable work |
| migration down with product/Shadow facts | `AGENT_PRODUCT_DOWN_DATA_EXISTS` |

### 5. Good / Base / Bad Cases

- **Good**: authenticated user reloads owned Runs/Schedules, administrator
  reviews exact Draft receipts, and default-off Shadow remains content-free.
- **Base**: Agent Center reports held Runtime; no legacy or Package execution
  occurs and ordinary Chat remains available without Skill prompt injection.
- **Bad**: browser receives object keys, API writes worker tables, a Shadow
  adapter runs package code in-process, or retired Conversation selection is
  accepted and projected back to a client.

### 6. Tests Required

- Focused backend race tests cover auth, ownership, approval decision/cancel,
  Schedule lifecycle, Draft review, exact mutation replay, Artifact seam, held
  enqueue, administrator checks and Shadow budget/Kill-Switch adapter fences.
- Frontend Vitest covers strict DTOs, URL reload, desktop/mobile composition,
  keyboard/focus/status, held/error paths and legacy retirement.
- PostgreSQL 17 proves fresh/replay, least privilege, ownership, cancel replay,
  Artifact lookup, default-off/cohort/opt-in, Kill Switch, budget, restart,
  generation/fingerprint fences, content-free dump/restore and guarded down/up.
- Every older Agent/MCP/Assistant/Skill tail drill peels empty `091`, then empty
  `090`, before its original guard and finishes at head `091`.
- Run Phase 0, backend vet/test, frontend full gate and full standalone. Exact
  host remains expected-nonzero until separately promoted.

### 7. Wrong vs Correct

#### Wrong

```text
enable Shadow -> API executes package -> copy result into Chat -> promote
```

#### Correct

```text
admin policy + user opt-in + exact cohort/fingerprints + Kill/budget fences
-> injected synthetic/read-only observation only
-> content-free durable fact -> no Chat/admission/promotion authority
```

## Scenario: Retire legacy text-Skill authority

### 1. Scope / Trigger

Apply when changing Chat Conversation metadata, frontend history schemas,
browser persistence version/migration, old Skill surfaces, or the G20.9 cutover
SQL. This scenario deletes old authority; it never converts or promotes it.

### 2. Signatures

- Backend guard: `internal/chat/legacy_skill_retirement.go`.
- Browser guard: `frontend/src/store/storage/legacySkillRetirement.ts` and
  `STORAGE_VERSION = 7`.
- History guards: `frontend/src/lib/api/schemas.ts` and
  `frontend/src/store/storage/migrations.ts`.
- Database operator cutover: `scripts/cutover-legacy-skills.sql` with
  `cutover_apply`, `expected_count` and `backup_fingerprint` psql variables.
- Gates: `verify-agent-legacy-cutover{,-postgres17}.sh`.

### 3. Contracts

- Create/update strips `activeSkills`; update also appends it to
  `MetadataDeleteKeys`. List/get/DTO projections strip it defensively so stale
  rows cannot revive client authority. Preserve Knowledge selection and all
  unrelated metadata.
- Browser purge removes exactly eight Settings fields plus Session/Workspace
  selections from top-level and nested Zustand envelopes. Marker-last,
  compensation and idempotent retry are mandatory.
- `skillInvocations` is input-only historical recognition. Output/runtime types
  expose only `legacySkillRetired?: true`; never retain invocation ID/title/
  description/category/mode.
- Database apply locks `conversations`, verifies full-backup SHA-256 and exact
  target count, updates only `metadata = metadata - 'activeSkills'`, verifies
  zero remaining, and leaves the current schema head at `091`. Rollback is full
  backup plus previous images; the cutover creates no migration or synthetic
  down SQL.
- No browser/API/rootful fallback executor is permitted. The only eligible
  Skill domain is admitted Package Skills, held at `ISOLATION_UNAVAILABLE` on
  an unqualified host.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| create/update contains `activeSkills` | key deleted before persistence; no error or conversion |
| stale row contains `activeSkills` | read projection omits key; update schedules server deletion |
| legacy invocation array is non-empty | message content retained; output becomes one retirement fact |
| SQL apply lacks valid backup fingerprint | `LEGACY_SKILL_BACKUP_FINGERPRINT_REQUIRED`; transaction rolls back |
| expected count differs from locked count | `LEGACY_SKILL_EXPECTED_COUNT_MISMATCH`; zero rows changed |
| exact host remains unavailable | `ISOLATION_UNAVAILABLE`; no legacy or fallback execution |

### 5. Good / Base / Bad Cases

- **Good**: backup/count are verified, apply removes one JSONB key, restart and
  reload show zero resurrection, and history renders one retirement label.
- **Base**: no stale database rows exist; expected count `0` applies
  idempotently and current schema head remains `091`.
- **Bad**: add a reversible migration that invents deleted values, preserve old
  invocation details, name-match a package, or silently execute in Chat.

### 6. Tests Required

- Go Handler regression covers create/list/update and repository state.
- Vitest covers raw/nested Settings/Chat purge, compensation, marker-last,
  Zustand migrate/partialize, Session/Workspace normalization and history
  collapse.
- PostgreSQL 17 covers default dry-run, rejected count/fingerprint, full backup
  fingerprint, exact-key update, unrelated-row equivalence, repeated apply and
  migration head `091`.
- Negative source scan proves removed files/assets/resolver/context and held
  Runtime; then run frontend/backend/full standalone gates.

### 7. Wrong vs Correct

#### Wrong

```text
old activeSkills/name/body -> find Package Skill -> install or execute -> keep details for rollback
```

#### Correct

```text
verified backup + exact count -> delete only retired authority -> one history fact
-> Package Runtime remains server-owned and held -> rollback only by full restore
```

## Scenario: Activate G21.0 control-plane maintenance

### 1. Scope / Trigger

Apply when changing the exact-host Runner bundle, staged activation evidence,
Runner client identity, `agent-runtime-control` command/service or recovery
reconcile behavior. G21.0 permits maintenance only and itself adds no
migration; the current release is nevertheless bound to the reviewed `091`
head introduced by G21.2.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-runtime-g21-0.sh
bash mm-chat/scripts/build-agent-runner-bundle.sh \
  --output DIR --release-commit HEX40
python3 mm-chat/scripts/verify-agent-runner-bundle.py --bundle DIR
python3 mm-chat/scripts/evaluate-agent-production-activation.py --record FILE
```

- Commands: `backend/cmd/agent-runtime-control`, `cmd/neo-runnerd` and
  `cmd/neo-runner-probe`.
- Packages: `internal/agentactivation`, `internal/agentruntimecontrol` and
  `internal/agentrunner`.
- Schemas: `neo-agent-production-activation.schema.json` and
  `neo-agent-runner-bundle.schema.json`.
- Database capability: existing `agent_runner_control` at migration head `091`.

### 3. Contracts

- The control binary is separate from `cmd/api`. It receives one PostgreSQL URL,
  exact private HTTPS Runner URL, Runner/server/client identities, client
  certificate/key, server CA, approved release manifest, operations policy,
  staged activation record, release commit and bounded timing/batch settings.
- Its LOGIN recursively inherits exactly `agent_runner_control`; any second
  membership or superuser/CREATEDB/CREATEROLE/replication/BYPASSRLS attribute
  rejects startup.
- Every cycle is `activation -> probe -> list -> PostgreSQL recovery inventory
  -> reconcile -> list/equality`. Health is the read-only
  `activation -> probe -> list -> inventory/equality` path. Equality includes
  Sandbox state as well as identity and all immutable fingerprints.
- Allowed RPCs are exactly `probe`, `list` and `reconcile`. The worker has no
  Step claim, launch authority, launch/heartbeat/cancel, Broker, Child, Cron,
  Learning, Chat or HTTP surface.
- Activation stage is exactly `control_plane`, <=24 hours old, approved,
  production-class and bound to migration `091`, release/policy/manifest/
  binary/deployment/endpoint/mTLS fingerprints. Only control authorization is
  true, `reviewedAt` is not in the future and orphan/Scratch residue is zero.
- Runtime evidence files are opened with no-follow semantics, bounded from the
  descriptor and parsed from the exact bytes that were fingerprinted; never
  reopen a manifest between hash binding and semantic validation.
- Client TLS validates TLS 1.3, exact server CA/name and client certificate CN
  equal to `spiffe://neo-chat/agent-runtime-control`. Runner URLs require an
  explicit numeric port in `1..65535`.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| control flag false | command exits before DB/file/Runner access |
| execution-stage flag true | configuration rejected |
| activation template/stale/drifted/widened/residue | held/invalid; zero DB or RPC call |
| future review or symlink/writable evidence | invalid before DB or RPC call |
| manifest unapproved/placeholder or mounted hash drift | activation invalid |
| DB login shared, privileged or has any second inherited role | startup rejected |
| probe not ready/missing exact feature | `AGENT_CONTROL_UNAVAILABLE`; no reconcile |
| expired/probe-drifted recovery row | excluded from expected set and reaped |
| post-reconcile identity/fingerprint/state differs | worker/health exits nonzero |

### 5. Good / Base / Bad Cases

- **Good:** fresh exact-target activation plus dedicated login reconciles only
  current PostgreSQL-authorized Sandboxes and proves equality.
- **Base:** all Agent flags are false; no target files are read and the current
  host remains `ISOLATION_UNAVAILABLE`.
- **Bad:** import control into API, reuse API/migrator credentials, accept a
  hostname/public/plaintext endpoint, call launch, or treat an offline fixture
  as live activation.

### 6. Tests Required

- Run focused race/vet tests for both control packages, Runner client and both
  Runner commands.
- Prove exact RPC order, activation-before-I/O, expired/probe drift cleanup,
  state-sensitive equality, health no-reconcile and forbidden method absence.
- Validate activation/bundle schemas and fixtures; test stale, widened,
  future-review, unapproved, placeholder, symlink, extra, missing,
  mode/size/hash and static ELF/architecture failures.
- Run preflight positive activation plus shared-principal, public endpoint,
  execution-flag, insecure file, certificate/key mismatch and non-READY cases.
- Run Phase 0 and full standalone; require the exact-host command to fail here
  with `ISOLATION_UNAVAILABLE`.

### 7. Wrong vs Correct

#### Wrong

```text
API process + API DB login -> unreviewed Runner URL -> launch package
```

#### Correct

```text
fresh control_plane evidence + dedicated agent_runner_control login
-> probe/list/recovery inventory -> reconcile -> exact equality
-> Root Run and every execution-stage flag remain false
```

## Scenario: Execute the G21.1 synthetic Root Run canary

### 1. Scope / Trigger

Apply when changing `agent-runtime-root-canary`, caller-specific Runner method
policy, Root-canary activation/plan validation, atomic terminalization or its
PostgreSQL role. G21.1 permits exactly one separately activated synthetic Root
Run and does not enable user-facing or general Runtime execution.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-root-canary-activation.sh
bash mm-chat/scripts/verify-agent-root-canary-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-1.sh
```

- Command: `backend/cmd/agent-runtime-root-canary`.
- Packages: `internal/agentrootcanary`, `internal/agentactivation`,
  `internal/agentrunner` and existing `internal/agentorchestrator`.
- Schemas: `neo-agent-root-run-canary-plan.schema.json` and
  `neo-agent-root-run-canary-activation.schema.json`.
- Database: existing migration `084`/`085` functions only. Migration `091` may
  be present at the current head, but G21.1 receives no Artifact role/function.

### 3. Contracts

- Use `spiffe://neo-chat/agent-runtime-root-canary`, separate canary mTLS/key
  files and a separate `LOGIN INHERIT` whose recursive roles are exactly
  `agent_orchestrator_runtime,agent_runner_control`. Reject elevated attributes,
  extra roles and direct table DML.
- Runner ingress authorizes methods by verified caller before execution.
  G21.0 control is only `probe/list/reconcile`; canary is only
  `probe/list/reconcile/launch/heartbeat/cancel`. Canary cannot call Broker
  `prepare/commit`.
- The immutable plan is synthetic, depth zero and content-free, with an empty
  Tool Registry, `networkMode=none`, no Egress/Secrets/Artifact publication,
  read-only rootfs, empty capabilities and bounded resources.
- The `root_run_canary` record binds release, target, policy, endpoint, exact
  canary certificate, plan and authority public key. All eight checks,
  approved production review and zero residue are required. G21.0 control must
  be enabled before canary preflight can pass.
- Signed launch/heartbeat/cancel authority binds request ID, nonce, canonical
  request fingerprint, user, Run/Step/Attempt/generation, owner, lease-token
  digest, snapshot, Runner and Kill-Switch epoch. Attaching a ticket must not
  regenerate request replay identity.
- Execute one idempotent enqueue/claim/launch flow, persist expected/running
  Sandbox state, exercise Runner and PostgreSQL heartbeats, cancel and require
  zero Runner inventory.
- In one SQL transaction, move Sandbox `running -> stopping -> terminal` and
  Attempt/Step/Run `running -> canceled`, appending one terminal event for each
  Orchestrator entity. Any identity, lease or expected-state mismatch rolls
  everything back.
- Terminal replay never launches again. A tokenless live Attempt waits for
  expiry and generation reclaim. Non-expired expected Sandboxes cause
  reconcile to fail closed until expiry rather than guessing authority.
- After a known-running heartbeat failure, first attempt exact signed cancel
  and atomic durable cancellation, then return unavailable. A cancel failure
  leaves durable state fenced for expiry/recovery.
- The command is not imported by `cmd/api`; the Compose profile defaults off,
  has no port or Provider/object-store/Redis/MCP/vault credentials, and every
  broad Runtime/Broker/Child/Cron/Learning flag remains false.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| canary false or G21.0 control false | no file/DB/Runner activation |
| plan unknown field, Tool, Egress, Secret or mutable setting | `ROOT_CANARY_PLAN_INVALID` |
| activation stale/template/drifted/widened/residue | held/invalid before launch |
| shared/elevated/extra-role LOGIN | startup rejected |
| control launch or canary Prepare/Commit | HTTP `403` before Runner execution |
| live tokenless Attempt | wait for lease expiry; no token reconstruction |
| Sandbox ID/state drift during terminal chain | transaction rollback; all projections stay running |
| heartbeat failure after running projection | attempt signed cancel/atomic terminalization, then fail cycle |
| nonzero post-reconcile inventory | canary unavailable; no success evidence |

### 5. Good / Base / Bad Cases

- **Good:** fresh exact-target activation runs one synthetic canary, atomically
  cancels every projection and proves zero Runner inventory.
- **Base:** both profiles and all broad flags are false; checked-in evidence
  remains `ISOLATION_UNAVAILABLE` and no Sandbox launches.
- **Bad:** reuse control identity, grant Broker authority, persist lease tokens,
  mark Run terminal outside the Sandbox transaction, or treat disposable
  PostgreSQL success as exact-host evidence.

### 6. Tests Required

- Focused race/vet for Root-canary command/service, activation, Runner,
  Orchestrator and `neo-runnerd`.
- Run Runner filesystem fixtures under `umask 022`; the production verification
  harness may otherwise force their intentional mode-0711 broker directory to
  0700 and create a false isolation failure.
- Explicit control-launch and canary-Prepare ingress denial before driver work.
- Service happy path, terminal replay, tokenless live lease, Runner/PostgreSQL
  heartbeat failure cleanup and zero-inventory proofs.
- PostgreSQL 17 exact-role and atomic terminal tests, including wrong Sandbox
  rollback with Attempt/Step/Run still running.
- Strict schemas/fixtures, preflight negative/positive, development/production
  Compose topology, G21.0 regression, Phase 0 and standalone full gate.
- Exact-host check remains separately expected-nonzero here with
  `ISOLATION_UNAVAILABLE`.

### 7. Wrong vs Correct

#### Wrong

```text
control certificate + broad Runtime flag -> launch -> three independent terminal writes
```

#### Correct

```text
fresh root_run_canary evidence + separate exact-role identity
-> one signed synthetic launch -> heartbeats -> signed cancel/reap
-> one atomic Sandbox/Attempt/Step/Run terminal transaction -> zero inventory
```

## Scenario: Execute the G21.2 read-only Broker and Artifact canary

### 1. Scope / Trigger

Apply when changing the G21.2 Broker canary command, Runner-to-Broker relay,
reviewed read executors, Artifact publication authority, migration `091`, or
`broker_artifact_canary` activation. This slice is synthetic-only and does not
enable API/Chat or general Agent Runtime execution.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-broker-canary-activation.sh
bash mm-chat/scripts/verify-agent-broker-canary-preflight.sh
bash mm-chat/scripts/verify-agent-artifact-publication-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-2.sh
```

- Command: `backend/cmd/agent-runtime-broker-canary`.
- Packages: `internal/agentbrokercanary`, `internal/agentbrokerrelay`,
  `internal/agentbroker`, `internal/agentactivation`, `internal/agentrunner` and
  existing Orchestrator/Runner repositories.
- Schemas: `neo-agent-broker-artifact-canary-plan.schema.json` and
  `neo-agent-broker-artifact-canary-activation.schema.json`.
- Database: migration `091_agent_artifact_publication` and NOLOGIN
  `agent_artifact_control`.

### 3. Contracts

- Use caller `spiffe://neo-chat/agent-runtime-broker-canary`, relay transport
  `spiffe://neo-chat/neo-runner-broker-relay` and a dedicated LOGIN whose
  recursive memberships are exactly `agent_orchestrator_runtime`,
  `agent_runner_control`, `agent_effect_control`, `agent_artifact_control`.
  Reject shared/elevated principals, extra roles, owner membership and table DML.
- Runner caller policies stay disjoint: control is `probe/list/reconcile`; Root
  canary adds `launch/heartbeat/cancel`; Broker canary additionally adds only
  `prepare/commit`. The private relay accepts only Prepare/Commit.
- Runner verifies the signed authority first. The relay independently verifies
  the original ticket, unsigned body fingerprint, caller, Runner, Attempt and
  plan binding before resolving Grant/Registry from the mounted plan. Runner
  receives no Broker database, S3, MCP, Provider or vault credential.
- The strict plan contains exactly five one-Run actions: bounded Project read,
  bounded Workspace read, one exact auth-none MCP read, bounded Artifact
  publication and one acknowledgement-loss read. Every Sandbox is read-only,
  capability-free and `networkMode=none` with no credential-bearing input.
- Registered read executors require `classification=read`, `idempotent=true`
  and automatic approval. File executors reject traversal, links, special files,
  root/fingerprint drift and byte overflow. MCP accepts only the pinned manifest
  server and exact read Tool. Receipts contain canonical fingerprints only.
- Prepare/Commit exact replay performs no second execution. Stale generation or
  lease is denied before executor/object access. A possible send with unknown
  status terminalizes Run/Step/Attempt as `outcome_unknown`; later replay only
  observes it and never dispatches under another key.
- `agent_artifact_control` receives authorize/attach function execution only.
  Both phases recheck committing intent, Tool/action, user, Run, Attempt
  generation/live lease, snapshot, Grant/Registry, revocation, Kill Switch,
  byte bound and media allowlist. Attach also binds deterministic object key,
  name, size and SHA-256 under locks.
- Artifact publication reads one bounded quarantine snapshot, verifies and scans
  those same bytes, writes object before row, deletes the new object on row
  failure and removes quarantine after success or rejection. Exact replay is
  idempotent; Artifact ID/name/object collisions fail closed.
- The command remains absent from `cmd/api`; all broad Runtime/Broker-mutation/
  Child/Cron/Learning flags remain false. Source/disposable passes and the
  checked-in fixture are not exact-host evidence.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| G21.0/G21.1 not ready or G21.2 flag false | no Broker canary file/DB/Runner access |
| caller/relay identity reused or method widened | reject before Runner/Broker work |
| relay ticket/body/plan drift | non-retryable relay denial; no Broker call |
| read Tool mutable/unknown/non-idempotent/unapproved | `GRANT_DENIED`; no executor |
| traversal/link/special file or root drift | reject without content disclosure |
| stale lease/generation/snapshot/Grant/Kill Switch | deny before executor/object access |
| Artifact row/name/object collision | fail closed; remove only newly written object |
| possible send cannot be resolved | terminal `outcome_unknown`; zero retry |
| current development host | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good:** fresh exact-target evidence runs the five synthetic actions through
  Runner and the private relay, publishes one bounded Artifact and leaves zero
  Runner/quarantine residue.
- **Base:** all profiles/flags are false; no canary material is read and the
  current host remains unavailable.
- **Bad:** call Broker directly as canary evidence, embed Broker in Runner,
  provide credentials to Sandbox, accept a generic MCP Tool, retry
  `outcome_unknown`, or grant direct Artifact DML.

### 6. Tests Required

- Focused race/vet for Broker canary command/service, relay, Broker, Runner,
  activation and Orchestrator packages.
- Relay method/caller/ticket/body/plan negatives and exact Prepare/Commit replay.
- Read executor traversal/link/special-file/bounds plus exact MCP allowlist.
- PostgreSQL 17 fresh/replay/exact-role/authorize/attach/collision/stale/Grant/
  Kill-Switch/dump-restore/guarded-down/up proof at head `091`.
- Same-byte scan, object-before-row compensation, quarantine cleanup and
  `outcome_unknown` no-retry tests.
- Strict schemas/fixtures, enabled/default preflight, development/production
  Compose credential boundaries, G21.1 regression, Phase 0 and standalone full.
- Exact-host acceptance stays separately nonzero here with
  `ISOLATION_UNAVAILABLE`.

### 7. Wrong vs Correct

#### Wrong

```text
Broker canary -> direct in-process Broker shortcut -> Sandbox receives MCP/S3 key
```

#### Correct

```text
fresh broker_artifact_canary evidence -> signed Runner Prepare/Commit
-> private mTLS relay re-verification -> plan-bound Broker executor
-> function-only Artifact authority -> canonical receipt / terminal no-retry
```
