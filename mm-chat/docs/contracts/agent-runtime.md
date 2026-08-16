# Neo Agent Runtime Executable Contract

Status: G20.1 supply-chain, G20.2 durable Orchestrator, G20.3 Runner, G20.4
brokered effects, G20.5 depth-1 Child delegation, G20.6 durable Cron scheduling,
G20.7 Draft-only learning, G20.8 Agent Center/default-off Shadow control, and
G20.9 legacy text-Skill hard retirement are implemented. Exact-host isolation
and production Runner/Broker/Child/Scheduler/Learning/Shadow promotion are held.
Those optional durable OCI paths remain disabled, while the current
single-server product executes installed Skills through the separate
[`local_direct` contract](./local-skill-runtime.md).

Ordinary Chat migration `096` advances the application schema head but does not
requalify this held OCI release. Exact-host activation and closure artifacts
remain pinned to their reviewed migration head `095`; all switches stay off.
Legacy control-plane tail drills now peel empty `096` before `095` and restore
the current database head to `096`.

## 1. Scope and hard gates

This contract defines the stable boundary among Go Durable Orchestrator,
PostgreSQL, non-root `neo-runnerd`, per-Run rootless OCI Sandbox and mediated
Tool/Egress/Secret/Workspace/Artifact services.

It does not gate the current Hermes-style Chat Skill path. `local_direct` needs
no Runtime Manifest or OCI image and intentionally makes no isolation claim.

In scope:

- Agent Skills package plus Neo Runtime Manifest admission;
- immutable Skill/runtime bundle, fingerprint and SBOM authority;
- durable Run/Step/Attempt state, lease, recovery and events;
- Capability Grant, Egress policy, Secret Broker and budgets;
- Runner capability probe, launch, heartbeat, cancel/kill and side effects;
- one-level Child Agents, Cron snapshots and Draft-only learning;
- authenticated Agent Center projections and held synthetic/read-only Shadow;
- legacy text-Skill cutover and rollback requirements.

Hard gate for the optional durable OCI path: no executor, route or feature flag may make a production Agent Run
until the owning G20 groups implement this contract and pass the Isolation
Acceptance Suite. `POST /v1/code/executions` remains
`CODE_EXECUTION_UNAVAILABLE`; it is not this Runtime API.

## 2. Contract artifacts

| Artifact | Version / file |
| --- | --- |
| Skill runtime declaration | `neo.skill-runtime/v1` — [`neo-skill-runtime-manifest.schema.json`](./schemas/neo-skill-runtime-manifest.schema.json) |
| effective authorization | `neo.capability-grant/v1` — [`neo-capability-grant.schema.json`](./schemas/neo-capability-grant.schema.json) |
| Runner protocol envelope | `neo.runner-rpc/v1` — [`neo-runner-rpc.schema.json`](./schemas/neo-runner-rpc.schema.json) |
| durable event | `neo.run-event/v1` — [`neo-run-event.schema.json`](./schemas/neo-run-event.schema.json) |
| immutable Cron revision | `neo.cron-template/v1` — [`neo-cron-template.schema.json`](./schemas/neo-cron-template.schema.json) |
| immutable learning Draft | `neo.skill-draft/v1` — [`neo-skill-draft.schema.json`](./schemas/neo-skill-draft.schema.json) |

JSON Schema Draft 2020-12 is the serialization authority. Unknown object fields
are rejected. Schema validation never replaces authorization or cross-document
checks. Valid/invalid fixtures live under
[`fixtures/agent-runtime/`](./fixtures/agent-runtime/).

## 3. Identity and immutable snapshot

Before `pending -> admitted`, Backend creates one canonical snapshot containing:

```text
snapshotId / snapshotFingerprint
userId / projectId / conversationId / assistant revision
rootRunId / parentRunId? / depth (0 or 1)
model provider + model ID + reasoning policy
Skill packageFingerprint + runtimeBundleFingerprint + admissionId
Tool Registry fingerprint
Capability Grant ID + canonical fingerprint
Egress rules + Secret refs
Workspace snapshot ID + content/version fingerprint
token / wall / Tool call / artifact / child budgets
createdAt + expiresAt + policy revision + Kill Switch epoch
```

`neo.runtime.json` is a package declaration, not a self-attestation. It
contains package name/version and the requested immutable runtime surface, but
never its own source coordinate, package/runtime/SBOM fingerprint, admission
ID or reviewer. The Backend derives those bindings from the complete package
bytes and stores them in the immutable version/admission envelope before any
snapshot can reference them.

Every Attempt and Runner RPC binds the exact snapshot and lease generation.
Changing a model, Skill, Runtime, Tool schema, grant, Egress rule, Secret ref,
Workspace snapshot, schedule or budget creates a new snapshot. Run snapshots are
never updated in place.

## 4. Durable state machine

### Run

```text
pending -> admitted -> queued -> running
pending|admitted|queued -> canceled|killed|failed
running -> succeeded|failed|canceled|killed|outcome_unknown
```

### Step

```text
pending -> ready -> running
pending|ready -> skipped|canceled|killed|failed
running -> succeeded|failed|canceled|killed|outcome_unknown
```

### Attempt

```text
leased -> starting -> running
running -> prepared -> committing
leased|starting|running|prepared -> failed|canceled|killed|lease_expired
committing -> succeeded|failed|killed|outcome_unknown
lease_expired -> [no transition; replacement Attempt gets a new ID/generation]
```

`prepared -> canceled` is allowed only before Commit is accepted. Once Commit
may have reached an external system, cancellation cannot assert rollback;
terminal state follows the Commit receipt or `outcome_unknown`.

Every transition requires an append-only event with expected prior state,
sequence and, for Attempt-related changes, lease generation. Invalid or stale
transitions return `INVALID_TRANSITION` or `LEASE_STALE` and create no state
mutation. Terminal events are immutable.

### Terminal precedence

When concurrent observations race, the projection uses:

```text
outcome_unknown > killed > canceled > failed > succeeded
```

This precedence is conservative, not a mechanism for rewriting terminal facts.
The first valid terminal append fences later attempts; reconciliation appends a
separate diagnostic fact when an incompatible observation arrives.

### Retry contract

- Admission, lease and launch failures before workload start may retry with a
  new Attempt if the error class is explicitly retryable and budget remains.
- Pure/read-only Tool actions may retry only if their Tool policy marks them
  idempotent and no result was observed.
- Mutable/unknown actions never retry a Prepare/Commit cycle with a different
  idempotency key.
- A stable Commit idempotency key may be queried/replayed only by the same
  frozen intent, grant, Attempt lineage and current valid lease.
- `outcome_unknown` is terminal and requires human reconciliation or an
  operation-specific status query; it never auto-commits again.

## 5. Lease and recovery

- Lease acquisition is a PostgreSQL transaction and increments a per-Step
  generation.
- Heartbeat extends the exact Attempt/generation only. It cannot resurrect an
  expired lease.
- Reclaim appends `lease.expired`, fences the old generation, then creates a new
  Attempt ID. Old output, Artifact, Prepare and Commit messages are rejected.
- `neo-runnerd` reconnect/restart lists exact live Sandbox/Attempt IDs. Backend
  kills any Sandbox without a current matching lease and snapshot fingerprint.
- Backend restart recovers from PostgreSQL; Redis is never needed to decide
  authorization, current state or cancellation.

G20.2 implementation signatures:

- `backend/internal/agentorchestrator/` is an internal typed service/repository
  seam; it has no HTTP/startup wiring;
- migration `084` owns immutable snapshots, current projections, append-only
  events, leases and revisioned Kill Switches;
- `agent_orchestrator_runtime` has read plus exact mutation/recovery function
  execution and no direct table DML; `go_api_runtime` has no G20.2 privileges;
- `scripts/verify-agent-orchestrator{,-postgres17}.sh` prove state/race/lease,
  restart/rebuild, stale denial, Kill Switch, retention and dump/restore while
  no Runner or Sandbox exists.

G20.3 implementation signatures:

- `backend/internal/agentrunner/`, `cmd/neo-runnerd` and
  `cmd/neo-runner-probe` implement the private Runner seam without API/startup
  wiring;
- migration `085` owns caller+Runner+request+nonce replay claims and expected
  Sandbox lifecycle projection; `neo-runnerd` has no PostgreSQL credential;
- `config/agent-runner/` and `deploy/agent-runner/` freeze the exact unapproved
  release, seccomp and rootless systemd contract;
- `scripts/verify-agent-runner*.sh` prove source/RPC/lifecycle, PostgreSQL and
  exact-host fail-closed behavior. Current host result is
  `ISOLATION_UNAVAILABLE`, not production evidence.

G20.4 implementation signatures:

- `backend/internal/agentbroker/` owns deterministic Registry construction,
  Prepare/approval/Commit coordination, Egress/Secret mediation and narrow
  Project, Artifact and MCP executor seams without HTTP/startup wiring;
- migration `086` owns immutable intents, append-only approvals, Commit
  claims/receipts, budget fences and secret-handle digests through the narrow
  `agent_effect_control` role;
- `backend/internal/safenet/` is the shared MCP/Agent DNS, address, redirect and
  response-limit policy boundary;
- Runner `prepare` and `commit` messages are strict authenticated relay shapes.
  The default relay remains unavailable and Runner receives no database, vault,
  object-store or MCP credential;
- `scripts/verify-agent-broker{,-postgres17}.sh` prove source/control,
  concurrency, least privilege, recovery, guarded rollback and dump/restore.
  They are not production isolation or mutable-executor evidence.

G20.5 implementation signatures:

- `backend/internal/agentdelegation/` derives server-owned Child Grants,
  Registries and snapshots, admits launch, settles reservations, and coordinates
  Child-first cascade/reap without HTTP/startup wiring;
- migration `087` owns immutable root/child authority, exact Parent Attempt
  lineage, transactional reservations, append-only settlements and durable reap
  work through the narrow `agent_delegation_control` role;
- Runner `launch` includes signed `runLineage` and locally rejects depth 2,
  root/parent drift and forbidden Child Tool identities before OCI create;
- `scripts/verify-agent-delegation{,-postgres17}.sh` prove subset, concurrency,
  lease/launch fences, terminal settlement, cascade, recovery, least privilege,
  guarded rollback and dump/restore. They do not enable production Child
  execution or supply exact-host isolation evidence.

G20.6 implementation signatures:

- `backend/internal/agentcron/` validates and fingerprints immutable template
  revisions, performs strict five-field schedule/IANA timezone calculation, and
  plans bounded occurrences without HTTP/startup wiring;
- migration `088` owns templates, immutable revisions, automation approvals and
  revocations, exact cursors, generation-fenced claims, unique UTC occurrences,
  atomic normal-Run links, sanitized audits and bounded cleanup through the
  narrow `agent_cron_control` role;
- the stable occurrence and Run idempotency identity is the exact template ID,
  revision and `scheduled_for` UTC instant. Enqueue acknowledgement replay
  returns the existing linked Run;
- `scripts/verify-agent-cron{,-postgres17}.sh` prove DST, bounded missed/overlap
  policies, concurrent/restart claims, stale fencing, authority revocation,
  pause/resume/delete, least privilege, guarded rollback and dump/restore. They
  do not expose a Cron API, start a Scheduler or enable production Runtime.

G20.7 implementation signatures:

- `backend/internal/agentlearning/` validates immutable Draft archives,
  evidence/tests/changed paths, version-only authority, bounded in-memory diff,
  administrator decisions and object-before-row cleanup without HTTP/startup
  wiring;
- migration `089` owns Drafts, exact check receipts, append-only decisions,
  promotion links, cleanup claims and sanitized audits through the narrow
  `agent_learning_control` role;
- only a same-user succeeded depth-0 source Run is eligible. Exactly one
  `static`, `isolation` and `evaluation` receipt for the live claim generation
  is required before human Promote;
- Promote rehashes and revalidates the exact quarantined archive and atomically
  inserts a new admitted `learning` candidate/package. Base archive validation
  must also reproduce the stored base package fingerprint. It does not edit a
  live package, installation, Run snapshot, Grant or Cron revision;
- `scripts/verify-agent-learning{,-postgres17}.sh` prove archive/provenance/
  policy checks, stale claims, human-only replay, collision drift, cleanup,
  least privilege, guarded rollback and dump/restore. Default isolation and
  evaluation adapters remain unavailable.

G20.8 implementation signatures:

- `backend/internal/agentcontrol/` is the authenticated product facade over
  existing Skill supply, Broker, Cron and Learning services. It exposes owned
  Run process data, bounded Artifact download, exact cancel/approval/Cron/Draft
  mutations and Shadow opt-in without worker, lease or Commit identity;
- migration `090` owns sanitized Agent Center views, append-only cancellation,
  bounded Artifact metadata and default-off Shadow policy/opt-in/observation
  authority. `go_api_runtime` receives exact views/functions, never direct
  worker table DML, claim, lease, Commit or scheduler-trigger authority;
- the top-level frontend Agent Center keeps Package Skills, Runs, Schedules and
  administrator Learning Review separate from Assistants and MCP. DTOs are
  strict Zod-validated and URL state preserves tabs/selection across reload;
- Shadow supports only `synthetic|read_only`, requires administrator policy plus
  user opt-in, binds cohort/revision/generation/boot/admission/package/runtime
  fingerprints and persists content-free observations. Exact-host unavailability
  returns `ISOLATION_UNAVAILABLE` and schedules no executable work;
- G20.8 produced deterministic local inventory, explicit raw local backup and a
  deletion dry-run with an empty execution set. G20.9 removes those temporary
  preparation surfaces together with the legacy editor/executor;
- `scripts/verify-agent-product-shadow{,-postgres17}.sh` prove product contracts,
  authorization/fences/budgets/restart, content-free dump/restore and guarded
  rollback. Every older PostgreSQL tail drill finishes at head `095`.

G20.9 implementation signatures:

- `frontend/src/store/storage/legacySkillRetirement.ts`, persistence version
  `7`, bounded history guards, and deleted editor/catalog/service/resolver code;
- `backend/internal/chat/legacy_skill_retirement.go` strips retired
  Conversation selection on create/update/read;
- `scripts/cutover-legacy-skills.sql` plus
  `verify-agent-legacy-cutover{,-postgres17}.sh` prove backup/count-gated
  one-key deletion without migration `091` or Runtime promotion.

G20.10 implementation signatures:

- `config/agent-runner/production-policy.json` freezes conservative capacity,
  root/Child/Cron budget, canary, retention, cleanup, metric-label and alert
  defaults without enabling a worker;
- `neo-agent-production-policy.schema.json` and
  `neo-agent-production-closure.schema.json` define strict, content-free policy
  and release-bound live evidence. `template` evidence can never promote;
- `scripts/evaluate-agent-production-closure.py` derives a deterministic
  `PROMOTION_READY`, `PROMOTION_HELD` or `PROMOTION_EVIDENCE_INVALID` decision
  without live access or mutation;
- `scripts/verify-agent-production-closure.sh` proves positive semantics only
  with an ephemeral synthetic production record and proves held, stale, drift,
  incomplete, duplicate and residue failure paths. Its committed exact-host
  template remains held at `ISOLATION_UNAVAILABLE`;
- G20.10 adds no migration, Runtime flag, Compose/startup worker, production
  adapter or fallback executor. Real promotion requires all 16 checks from the
  exact target deployment and keeps the resulting sanitized record outside Git.

## 6. Runner RPC

### Transport

- private endpoint only; no public/host-wide unauthenticated listener;
- mutually authenticated service identity with pinned trust roots;
- TLS 1.3 exact; no plaintext or bearer-token fallback;
- exact protocol version negotiation before `ready`;
- bounded body, deadline and concurrent request limits;
- `requestId + nonce + caller identity + authority-request fingerprint` durable
  replay fence with a bounded TTL; the signed ticket carries the same nonce and
  fingerprint so changing its body invalidates both fences;
- responses echo request ID and carry no raw Secret, prompt, Tool result or
  Workspace content.

G20.4 activates strict `prepare` and `commit` relay shapes in addition to the
G20.3 `probe`, `launch`, `heartbeat`, `cancel` and `list` methods. The relay
validates method-specific arguments, canonical argument fingerprint, signed
authority fingerprint and local replay claim, then calls an injected Broker
interface. Its default implementation returns `RUNTIME_UNAVAILABLE`; there is
no production Backend channel or effect execution. Method/body mismatch,
unknown field, duplicate key, expired
nonce, stale lease, fingerprint mismatch or unsupported version fails closed.

### Capability probe

Required release features:

```text
rootless_userns
cgroup_v2
seccomp
readonly_rootfs
snapshot_workspace
network_none
subordinate_ids
pidfd_kill
cgroup_reap
```

Probe evidence binds runtime binary/version, kernel, user namespace/subuid
mapping, cgroup delegation, seccomp availability, network driver, storage
driver and a probe fingerprint. Docker daemon availability does not satisfy it.
Missing or drifted evidence returns `ISOLATION_UNAVAILABLE` and keeps readiness
false.

### Launch hard constraints

- `neo-runnerd` process is non-root and does not use setuid/sudo to execute Runs.
- container user UID/GID are nonzero inside the namespace;
- rootfs/package/runtime are immutable/read-only;
- `no_new_privs=true`, capability set empty, approved seccomp exact fingerprint;
- cgroup v2 CPU/memory/PID limits and wall/output deadlines are nonoptional;
- no host PID/IPC namespace, Docker/Podman socket, device, database/object-store
  credential or arbitrary host mount;
- `networkMode=none` or an approved brokered path only;
- Project data is a bounded snapshot rehashed before every launch; Scratch is
  a size-bounded noexec/nosuid/nodev tmpfs; Artifact uses a per-Attempt local
  Unix socket into Runner-owned quarantine and never object storage;
- OCI image and every executable/dependency are digest/fingerprint bound.

## 7. Skill package and admission

The package root has standard `SKILL.md`; executable Skills also carry
`neo.runtime.json`. `SKILL.md` `allowed-tools` is not effective authorization.

Admission stages:

1. record source adapter and immutable upstream coordinate;
2. fetch to a no-execute quarantine with size/time limits;
3. reject archive traversal, link escape, special files, duplicate/colliding
   paths, decompression bombs and unsupported encodings;
4. validate Agent Skills frontmatter and Neo Manifest;
5. resolve Git commits, images and dependencies to immutable digests;
6. inventory canonical path+byte hashes and generate SBOM;
7. build/scan Runtime Bundle without injecting deployment secrets;
8. run static checks and an isolated admission test profile;
9. show exact source/fingerprints/SBOM/capability/egress/secret requests to a
   human administrator;
10. persist admit/reject decision and immutable objects.

Changing any package byte, manifest declaration, dependency, runtime image,
entrypoint or SBOM input invalidates the admission. LobeHub display metadata,
Git tags, ZIP filenames and official-source labels never override fingerprints.

## 8. Capability Grant

The effective Grant is a Backend-signed/canonically fingerprinted allowlist
bound to subject, exact Run/depth, admitted package/runtime fingerprints,
expiry, capabilities, Egress, Secret refs and budget.

Authorization for each action is:

```text
current authenticated subject
AND exact frozen snapshot/grant/package/runtime
AND action/resource selector match
AND current admission/revoke/expiry/Kill Switch
AND remaining call/time/token/artifact budget
AND required approval state
AND live Attempt lease generation
```

Denial at any term is terminal for that call. The Sandbox/model cannot present
a different Grant, extend expiry or request a new Broker reference. Grant
management is never present in a Child Registry.

### Egress

- `none`: no network namespace path to external destinations.
- `allowlist`: Broker permits exact `https`/`wss` host+port rules only.
- `brokered`: Tool-specific service owns request construction and response caps.
- raw IPv4/IPv6 literals, wildcard hosts, localhost, private/link-local/
  multicast/metadata destinations and alternate numeric forms are rejected.
- DNS resolution is rechecked/pinned per connection; redirects and reconnects
  repeat policy. Credentials are never forwarded across an origin change.
- request/response bytes, time, redirect and concurrency are bounded.

### Secret Broker

- Grant references an opaque `secret_ref`; Sandbox never receives vault bytes.
- Broker resolves it for the exact subject/Run/Attempt/capability/action/
  destination and issues the narrowest possible short-lived handle.
- PostgreSQL validates the exact `committing` intent, live lease generation and
  current Kill Switch both before vault resolution and before handle consume.
- Handles are non-renewable by Sandbox, single-action where possible, and
  revoked on terminal/kill/lease expiry; Commit completion revokes all remaining
  active handles before returning its durable receipt.
- Secret values/handles never enter prompt, environment, argv, Workspace,
  Scratch persistence, Artifact, Tool output, event, log or metric.
- If a third-party protocol requires a credential, the Broker adds it outside
  Sandbox and strips it from returned data.

## 9. Workspace, Scratch and Artifact

- Workspace input is an immutable Project snapshot with owner/version/content
  fingerprint. It is not a host project bind mount.
- Proposed Project changes become a bounded patch intent. Prepare verifies path,
  file type, size, scope and base revision; Commit uses CAS plus idempotency.
- Scratch is private to one Run, quota-limited and removed on every terminal,
  kill, orphan reconcile and Runner restart cleanup path.
- Artifact bytes stream to Backend/Broker without object-store credentials.
  Backend enforces quota, media/type policy, secret/malware scan, owner binding,
  exact size plus SHA-256 over one bounded byte snapshot, and object-before-row
  deletion. The scanner and object write consume the same verified bytes so a
  quarantine replacement cannot change the published content.
- Symlinks, hardlinks, devices, sockets, FIFOs and path escape never cross from
  Sandbox into Project or Artifact storage.

## 10. Prepare / Commit protocol

`prepare` returns an immutable intent ID/fingerprint, requested approval class
and expiry after validating current grant, arguments fingerprint and resource.
Prepare must not perform the external mutation.

Backend persists the prepared intent and any user approval. `commit` rechecks
the same intent fingerprint, lease generation, approval, Kill Switch,
revocation, expiry and budget. It then uses one stable idempotency key.

`cancel` is an authenticated user/operator action bound to an append-only
cancellation ID plus the exact intent fingerprint. It can advance only an
`awaiting_approval` or `approved` intent and its `prepared` Attempt. PostgreSQL
locks the same intent for Cancel and Commit, so exactly one wins. Exact Cancel
replay is stable; moving its ID, actor or reason is `REPLAY_DETECTED`. A Commit
winner rejects cancellation and remains governed by receipt/`outcome_unknown`.

Grant revocation is a separate append-only authority bound to the exact Grant
fingerprint, actor and reason. Prepare, Commit, Secret creation and Secret
consumption all recheck it. Revocation changes no completed receipt; it revokes
active durable handle digests and the coordinating control service zeroizes
matching memory-only Secret bytes.

| Crash/race point | Required result |
| --- | --- |
| before Prepare is persisted | no effect; new Attempt may prepare again |
| after Prepare, before approval | intent remains pending/expirable; no effect |
| after approval, before Commit accepted | cancel/kill wins; no effect |
| Commit accepted, before external send | same key may safely resume if executor proves no send |
| external effect succeeds, acknowledgement lost | query exact idempotency receipt; if unknowable, `outcome_unknown` |
| cancel races after external send | never claim canceled rollback; use receipt or `outcome_unknown` |
| stale lease sends Commit | hard reject; never forward externally |
| duplicate Commit with exact key/intent | return original receipt (`replayed`), never duplicate effect |
| duplicate key with different intent | hard reject and security audit |

## 11. Child Agent contract

- root is `depth=0` without `parentRunId`; child is `depth=1` with exact Parent;
  schema rejects depth 2+.
- A separate authenticated control `UserID` selects the durable Parent; proposed
  Grant subject fields never provide caller authority. Root authority binds
  exact user/Project/Assistant, snapshot, model,
  package/runtime, Grant, Registry, expiry and budget. Child subject/model/
  package/runtime are exact bindings; Tools, actions/resources, approval,
  Egress, Secrets, expiry and budgets are strict subsets/intersections.
- Enqueue binds the exact live Parent Attempt generation, lease owner and token
  digest. PostgreSQL locks the Parent and reserves wall seconds, model tokens,
  Tool calls and Artifact bytes atomically with Child Run creation. Exact replay
  is free; mismatched replay fails.
- Registry Builder always removes `delegate_task`, `cron_manage`,
  `grant_manage`, `secret_manage`, `runtime_manage` for depth 1 before computing
  the registry fingerprint.
- A retained Child Tool identity preserves its Parent capability,
  classification and idempotency class while actions, resource selectors,
  approval and call limits may only narrow. PostgreSQL prefix comparison uses
  literal `starts_with`, never wildcard `LIKE` semantics.
- Runner launch schema also rejects a Child registry containing those names;
  Backend/PostgreSQL admission repeats the check against the exact durable
  identity set and rechecks both live leases, fingerprints, expiry and Kill
  Switches.
- Settlement requires an already terminal Child Run, is exact-replay
  idempotent, cannot exceed its reservation, and releases only the reservation
  remainder proven by terminal usage.
- Parent cancellation/kill atomically fences every live Child lease and
  terminalizes Child Attempt/Step/Run state before reaping. Failed reaps remain
  durable; reconciliation discovers terminal, expired, reclaimed or
  Kill-Switched Parents and retries without restoring a lease.
- Child cannot delegate, create Cron, promote Drafts, approve its own effect or
  modify grants/secrets/runtime authority. No public or startup path activates
  this foundation in G20.5.

## 12. Cron and Draft learning

Cron accepts exactly standard five-field expressions. Seconds, descriptors and
embedded `TZ=` / `CRON_TZ=` are invalid. Timezone is a separate frozen IANA
name; the calculator is `robfig-cron/v3.0.1+go-tzdata`. DST gaps do not invent a
wall time and DST overlap produces both distinct UTC instants.

Each immutable template revision freezes exact owner, input reference and
fingerprint without body content, schedule/timezone/calculator, model, four
budgets, Skill installation/admission/package/runtime, Grant ID/fingerprint/
capabilities, Registry fingerprint, Egress, Secret refs, expiry, steps, scopes
and scheduling policies. The append-only approval class is
`automation_read_only` or `automation_brokered_effect`; brokered automation may
create a Run but cannot bypass per-effect Prepare/Commit. Editing any frozen
field creates a new revision and approval.

PostgreSQL stores the exact next cursor and fences cursor/trigger claims by
owner, generation and expiry. Missed policy is `skip`, `fire_once` or bounded
`catch_up` with at most 100 Runs and a 24-hour window. Overlap is `skip`,
`buffer_one` or `allow`; replacement/cancel and unbounded BufferAll do not
exist. Pause blocks materialization, resume moves to the next future instant
without backfill, and delete remains a tombstone until bounded cleanup.

Every trigger repeats current active revision, owner, exact installed/admitted
Skill, unrevoked Grant and automation approval, expiry, all applicable Kill
Switch scopes and overlap under database locks. A mismatch becomes a stable
sanitized skipped fact and never falls forward to newer authority. The
occurrence link and normal Orchestrator Run enqueue commit atomically. Retry is
allowed only while enqueue is unproven and retains the exact occurrence and Run
identity; it never retries an effect.

Learning output is an untrusted immutable `neo.skill-draft/v1` document and
content-addressed archive. It binds a same-user succeeded depth-0 Run and exact
snapshot, an existing base package, a distinct proposed package, unchanged
runtime authority, runtime/SBOM/archive/evidence/test fingerprints, exact tests,
changed paths and bounded source-package/Run-event evidence. Every changed path
must be covered by Run evidence. Durable documents contain no prompt/input,
Run output, Tool body, Secret/credential value or Workspace body.

Checking is claimed by owner/generation/expiry and publishes exactly one each
of `static`, `isolation` and `evaluation` for that generation. High-confidence
prompt override, secret copy, source laundering or evaluation gaming fails the
Draft. Policy failure cannot be retried in place; infrastructure failure is
bounded and stale workers cannot publish or release. Expired checking/cleanup
claims pass through bounded reconciliation before ready work can be claimed
again, so every crash consumes an attempt and exhaustion terminalizes.

Only the configured authenticated administrator may Reject or Promote with the
exact revision, Draft fingerprint, proposed package fingerprint and reason.
Exact replay returns the original decision. Promote refetches/rehashes bytes,
reruns complete package and authority validation, verifies the three exact
passing receipts and current source/Kill-Switch authority, then inserts one new
immutable admitted `learning` candidate/package. Automated check actors cannot
admit, install, Promote, mutate Grant/Secret/Runtime authority or change live
Runs/Cron. Rejected/promoted quarantine objects are deleted before cleanup-row
acknowledgement; a later exact Promote replay reads the append-only decision and
admission link without requiring deleted quarantine bytes. Cleanup/reconcile/
prune remain callable while Learning is off.

## 13. Agent Center and held Shadow

### Product facade

`/v1/agent-center/*` is authenticated. User reads always bind the session user
to Run, Step, Attempt, event, approval, Child, Artifact and Schedule
projections. Learning Draft list/detail/diff/review and Shadow policy mutation
require the configured administrator. The existing `/v1/skills/*` Store and
library remain the Package Skill authority; Assistant, MCP and legacy text
Skills are separate products.

Mutable requests carry their owning exact authority:

- Run cancel/kill: stable cancellation ID, expected live Run state and snapshot
  fingerprint;
- approval: intent/argument fingerprints, expected approval revision and exact
  `approved|denied` decision;
- Schedule create/lifecycle: immutable spec fingerprint, expected revision,
  stable request/audit identity and effective instant;
- Draft Reject/Promote: Draft revision/fingerprint, proposed package fingerprint
  and append-only human decision identity;
- Shadow: expected policy revision or user opt-in generation plus stable reason.

Handlers call owning services/functions; they never issue direct table DML or
claim Runner, Broker Commit, Scheduler trigger or learning-check authority.
Stale or cross-user mutations change no durable authority. Artifact list DTOs
omit object keys. Download resolves the key server-side from the exact user,
Run and Artifact tuple and returns a bounded authenticated stream.

The frontend validates every Agent Center response with strict Zod schemas.
Unknown/malformed payloads become `INVALID_SERVER_RESPONSE`. URL parameters
`panel=agent-center`, `agentTab` and `agentId` own reload/back/forward state.
Desktop list/detail and mobile drill-in expose loading, empty, error and held
states with keyboard operation, focus restoration and live status text.

### Shadow authority

Shadow defaults off and `effective` remains false in G20.8. An eligible cohort
requires all of: enabled administrator policy, unexpired policy window, exact
admitted package/runtime fingerprints, deterministic selected user, latest
explicit opt-in, current boot epoch/generation and unused observation/error
budget. The only modes are `synthetic` and `read_only`; neither grants writes,
Secrets, Egress, delegation, Cron creation or Draft promotion.

The injected adapter is a contract-replay seam, not an in-process package
executor. Missing exact-host isolation returns `ISOLATION_UNAVAILABLE` before
any work is scheduled. Opt-out, restart, expiry, budget, Kill Switch or
fingerprint drift fences new observations; stale generations cannot publish.
Persisted observations contain only boot/policy/generation IDs, admission and
package/runtime fingerprints, mode/outcome/reason, latency bucket and bounded
counts. Shadow results never enter Chat, install/admission or promotion
authority.

### Legacy inventory

G20.8 locally inventories the eight legacy settings fields as counts,
normalized IDs, fingerprints, invalid/orphan counts and the exact settings key.
The explicit backup may contain the raw local settings value because it is a
user-requested local export; it is never uploaded or logged. The deletion plan
is pure dry-run, deletes no storage key and proves Assistant, MCP, Chat,
Conversation, file, Knowledge and Memory domains are preserved. G20.9 alone
may execute deletion.

## 14. Kill Switch contract

Kill Switches are durable denies with actor, scope, mode, reason, revision and
timestamps. Broader deny overrides narrower state. Modes:

- `deny_new`: no new admission, Run, lease or action in scope;
- `cancel`: fence new mutable actions and request cooperative stop;
- `kill`: fence leases immediately and kill exact Sandbox cgroup/process group.

Every admission, lease, heartbeat, Broker action, Prepare and Commit checks the
current switch epoch. Runtime disabled or Runner unavailable never stops
retention, artifact cleanup, expired-intent cleanup, orphan reconciliation or
audit access.

## 15. Error matrix

| Condition | Stable code | Retry |
| --- | --- | --- |
| Runtime feature/probe missing | `ISOLATION_UNAVAILABLE` | only after operator repair/new probe |
| Runner mTLS/version failure | `AUTH_FAILED` / `VERSION_UNSUPPORTED` | no silent fallback |
| repeated/expired RPC nonce | `REPLAY_DETECTED` | new request only after authority recheck |
| snapshot/package/grant drift | `SNAPSHOT_MISMATCH` | new Run snapshot required |
| action outside Grant | `GRANT_DENIED` | no |
| expired/reclaimed Attempt | `LEASE_STALE` | old Attempt no; new Attempt by orchestrator only |
| applicable Kill Switch | `KILL_SWITCH_ACTIVE` | no while active |
| illegal state edge | `INVALID_TRANSITION` | no |
| external result unknowable | `OUTCOME_UNKNOWN` | never automatic Commit retry |
| Runner cannot start safely | `RUNTIME_UNAVAILABLE` | bounded new Attempt if classified retryable |
| Cron cursor/trigger owner or generation is stale | `STALE_CLAIM` | old claim never; reclaim with a new generation only |
| Cron revision changed after claim | `STALE_TEMPLATE` | no; materialize under the newly approved revision |
| Cron owner/Skill/Grant/approval/expiry/Kill authority denied | stable sanitized denial reason | no authority substitution; future occurrence only after explicit repair |
| Draft source/snapshot/package drift | `SOURCE_RUN_INVALID` / `SOURCE_DRIFT` / `PACKAGE_ALREADY_EXISTS` | new Draft from current authority only |
| Draft object or immutable-key collision drift | `DRAFT_OBJECT_DRIFT` | no overwrite or promotion |
| Draft check infrastructure unavailable | `CHECK_UNAVAILABLE` | bounded claim retry; policy failure is not retried |
| stale Draft check/cleanup worker | `STALE_CLAIM` | reclaim under a new generation only |
| Draft Promote before exact checks/human authority | `CHECKS_INCOMPLETE` / `PROMOTION_DENIED` | no automatic fallback |
| cross-user/non-admin Agent Center operation | `NOT_FOUND` / `ADMINISTRATOR_REQUIRED` | no authority change |
| malformed Agent Center response | `INVALID_SERVER_RESPONSE` | reload; never trust partial DTO |
| Shadow disabled, opt-out, cohort miss or exact host unavailable | stable held reason / `ISOLATION_UNAVAILABLE` | no executable work |
| stale Shadow policy/generation/boot/fingerprint | `REVISION_CONFLICT` / `GENERATION_STALE` | reload exact authority |
| Shadow observation/error budget exhausted | `BUDGET_EXCEEDED` | no new observation until a new policy |
| applicable Shadow Kill Switch | internal `KILL_SWITCH_ACTIVE`, HTTP `AGENT_AUTHORITY_DENIED` | no adapter/observation |

No error includes secret, raw arguments/result, package contents, Workspace
paths/content, host paths or provider payloads.

## 16. Isolation Acceptance Suite

Before production enablement, test the exact release artifacts on the target
host from a clean baseline:

1. probe non-root daemon, user namespace mappings, cgroup v2 delegation,
   seccomp, storage/network driver and exact versions;
2. inspect OCI config: nonzero UID/GID, read-only rootfs, empty capabilities,
   no-new-privileges, approved seccomp, bounded cgroup/PIDs/output/time;
3. prove no host PID/IPC, project bind, runtime socket, database/object-store/
   provider credential or arbitrary device is reachable;
4. negative filesystem cases: `..`, absolute paths, symlink/hardlink races,
   procfs/fd tricks, special files, Unicode/case collision and oversized trees;
5. negative network cases: DNS rebinding, redirect/reconnect, alternative IP
   notation, localhost/private/link-local/metadata and origin credential leak;
6. secret canary proof across env, argv, `/proc`, prompt, stdout/stderr,
   Workspace, Scratch residue, Artifact, logs/events/metrics;
7. CPU/memory/PID/disk/output/time exhaustion terminates exact Sandbox and
   leaves Backend/Runner responsive;
8. cancel/kill/Runner crash/host reboot/orphan cases reap process descendants
   and Scratch, then reconcile durable state;
9. lease reclaim rejects all stale output/Artifact/Prepare/Commit paths;
10. Prepare/Commit crash matrix proves idempotency and `outcome_unknown`;
11. Child depth 2, forged Parent, widened Grant/budget and forbidden
    `delegate_task` registry all fail before launch;
12. disabled Runtime still runs retention, cleanup and orphan reconciliation.

Passing requires machine-readable evidence bound to host/runtime/runner image,
runtime bundle, seccomp and test-suite fingerprints. A generic “container ran”
or Docker daemon health result is not acceptance.

## 17. Legacy text-Skill cutover

G20.9 deletes, rather than migrates or wraps:

```text
installedSkills / customSkills / activeSkillIds / skillAutoSelect
Conversation.config.activeSkills / Workspace.activeSkills
browser Skill selection and system-context assembly
legacy catalog/custom Skill definitions and execution references
```

Historical message content is preserved, but old `skillInvocations` are
collapsed at API/browser normalization boundaries to the boolean fact
`legacySkillRetired: true`. The UI renders only “旧版技能已退役”; it retains no
legacy ID/title/description/category/mode and cannot reopen or execute a
definition.

Browser persistence version `7` removes the eight retired settings fields and
Session/Workspace selection from top-level and nested Zustand envelopes in
localStorage and IndexedDB. The completion marker is written last; a failed
write compensates already-written records and a reload safely retries. Settings
and Chat `partialize`/normalization cannot recreate the fields.

The Go Chat boundary strips `activeSkills` on create/update/read. Existing
PostgreSQL rows are handled outside the migration chain by
`scripts/cutover-legacy-skills.sql`: dry-run is the default, apply requires the
exact target count and a `sha256:<64 lowercase hex>` full-backup fingerprint,
locks `conversations`, removes only `metadata.activeSkills`, verifies zero
remaining rows and commits. The cutover itself creates no migration or pretend
down SQL; current migrations `091`-`093` are unrelated Artifact, synthetic
Project-canary and Child-reap authority. Rollback
restores the matching full database backup and previous images as one operation.

G20.8 backup/inventory evidence must be captured before deploying the G20.9
browser build. Package Skills remain the only eligible Skill execution domain,
but this host is still honestly held at `ISOLATION_UNAVAILABLE`; browser/API
fallback execution is forbidden.

## 18. Production closure contract

The versioned operations policy is hash-bound into every closure record. Its
defaults never widen a frozen Grant: the initial single-server ceilings are two
concurrent root Runs, four Sandboxes, a queue depth of 32 and one canary. Root
budget is 300 wall seconds, 20,000 model tokens, 32 Tool calls and 16 MiB of
Artifact bytes; Child/Cron defaults are strict subsets. The first canary is
`synthetic|read_only`, no-Egress, no-Secret and allows zero
`outcome_unknown`.

A production record binds one immutable release to exactly these checks:

```text
exact-host isolation; clean-copy; restart; host reboot; paired backup/restore;
disaster recovery; rollback/forward-fix; hierarchical Kill Switch;
credential/mTLS rotation; Runtime Bundle rotation; orphan reconciliation;
outcome_unknown workflow; metrics/alerts; capacity/budgets; bounded canary;
temporary evidence cleanup
```

Every check stores only a result code, UTC time and SHA-256 of external
evidence. The record accepts no log body, prompt, Tool argument/result,
Workspace/Skill/Artifact content, path, URL, object key, credential, token or
Secret. Production readiness additionally requires a current review window,
non-placeholder release/target/reviewer bindings, all checks `passed`, review
`approved`, and zero temporary canary Run/Draft/Artifact/raw-Run/Sandbox/
Scratch residue while the sanitized record remains retained.

An `outcome_unknown` result is never repaired by changing the original terminal
Run/Attempt/receipt. Operators query only the exact non-mutating idempotency
status, retain ambiguity if it remains unknowable, and bind an append-only
content-free incident resolution digest into a later closure check. Generic
terminal retention must not select unresolved incidents.

The evaluator is read-only and fail closed. Exit `0` means the supplied
`production` document satisfies this contract but does not activate Runtime;
exit `3` is an honest held decision, including `ISOLATION_UNAVAILABLE`; exit
`2` is invalid evidence. The offline self-test's ephemeral positive case is not
live evidence and is deleted on exit.

## 19. G21.0 control-plane activation

The exact-host bundle schema is
`schemas/neo-agent-runner-bundle.schema.json`. Its seven payloads are exactly the
two static Runner binaries, release manifest, seccomp policy, systemd unit and
environment template plus the operator README. The bundle verifier rejects a symlink at any payload,
unexpected directory/file, missing file, mode/size/hash drift, dynamic ELF
interpreter and an unapproved or placeholder `production` release. Template
builds are deterministic and never install or mutate host state.

The stage record is `neo.agent-production-activation/v1`, stage
`control_plane`. It binds the exact Git commit, current migration head `095`, operations
policy, Runner manifest/binary, deployment, private HTTPS endpoint, Runner and
TLS identities, client certificate, server CA and these five unique live
checks:

```text
exact_host_isolation; clean_copy_install; private_mtls_probe;
zero_inventory_reconcile; rollback_kill_switch_ready
```

Only `authorization.controlPlane` is true. Root Runs, Broker read/mutation,
delegation, Scheduler and Learning are explicitly false. Evidence is
production-only, approved, no older than 24 hours and has zero orphan/Scratch
residue. Template, stale, incomplete, widened, placeholder or drifted evidence
holds or fails; the record cannot self-assert a verdict.

The dedicated worker revalidates the record and mounted policy/manifest/mTLS
hashes on every cycle, then runs `activation -> probe -> list -> PostgreSQL
recovery inventory -> reconcile -> list` and requires exact post-reconcile
equality. Its healthcheck performs the same read-only gate/probe/list/inventory
comparison without reconcile. The database login must inherit
`agent_runner_control` and must not inherit owner, API, effect, delegation,
Cron or Learning roles. Startup and cycle drift terminate the process.

G21.0 itself adds no migration, API/Chat route, launch/heartbeat/cancel RPC,
Broker adapter or public endpoint. Its current evaluator nevertheless binds
the current `095` head introduced by G21.6. All execution-stage environment
switches remain false, and the control profile defaults off.

## 20. G21.1 synthetic Root Run canary

The strict plan schema is
`schemas/neo-agent-root-run-canary-plan.schema.json`. It admits exactly one
synthetic user, one `root_canary` Step at depth zero, an empty Tool Registry,
`networkMode=none`, no Egress, no Secrets, no Artifact publication, a read-only
rootfs, empty capabilities and bounded resource limits. Unknown fields,
placeholders, mutable environment settings, additional argv entrypoints or any
Tool/Secret/Egress authority invalidate the plan.

The activation schema is
`schemas/neo-agent-root-run-canary-activation.schema.json`, stage
`root_run_canary`. It binds the exact G21.0-ready release/target/policy,
private Runner endpoint, canary certificate identity, server CA, immutable
plan and Ed25519 authority public key. Its exact authorization vector enables
Root Runs only; control-plane, Broker read/mutation, delegation, Scheduler and
Learning remain false. All eight unique checks must pass inside a maximum
24-hour production window with approved review and zero orphan/Scratch
residue. The checked-in fixture remains held by `ISOLATION_UNAVAILABLE`.

Runner ingress enforces caller-specific methods before execution:

```text
spiffe://neo-chat/agent-runtime-control
  -> probe, list, reconcile
spiffe://neo-chat/agent-runtime-root-canary
  -> probe, list, reconcile, launch, heartbeat, cancel
```

Neither identity may call a method outside its row; in particular control
cannot launch and the canary cannot call Broker `prepare` or `commit`. Launch,
heartbeat and cancel tickets bind caller, Runner, request ID, nonce, canonical
request fingerprint, user, Run/Step/Attempt/generation, lease owner/token
digest, snapshot and current Kill-Switch epoch. `BindAuthority` attaches the
ticket without regenerating request replay identity.

The worker idempotently enqueues and claims the plan's single Run, launches one
Sandbox, persists expected/running projection, transitions the Attempt to
running, exercises Runner and PostgreSQL heartbeats, sends a signed cancel and
requires zero Runner inventory after reconciliation. Final Sandbox
`running -> stopping -> terminal(canceled)` plus Attempt, Step and Run
`running -> canceled` and their three append-only events commit in one
PostgreSQL transaction. Any Sandbox/lease/state mismatch aborts the whole
transaction.

Replaying a terminal plan returns the same canceled Run without another
launch. A nonterminal Attempt whose memory-only token was lost cannot be
reconstructed or canceled by guess; it remains fenced until lease expiry,
after which ordinary reconcile and generation reclaim apply. A known running
projection that fails either heartbeat first attempts the exact signed cancel
and atomic cancellation, then returns unavailable. If cancel itself is
unavailable, durable live state remains fenced for expiry/recovery rather than
being falsely terminalized. Reconcile retains non-expired expected Sandboxes
but still requires zero final inventory, so a replacement worker fails closed
until that lease expires.

The dedicated LOGIN must be a nonprivileged `LOGIN INHERIT` principal whose
recursive membership set is exactly
`agent_orchestrator_runtime,agent_runner_control`. G21.1 reuses migration
`084`/`085` SECURITY DEFINER functions and itself adds no migration or direct
table DML; its current release gate accepts only the reviewed `095` tail. The
command and Compose profile are separate from `cmd/api`, have no
port or Provider/object-store/Redis/MCP/vault credentials, and default off.

Required focused gates are:

```bash
bash scripts/verify-agent-root-canary-activation.sh
bash scripts/verify-agent-root-canary-postgres17.sh
bash scripts/verify-agent-runtime-g21-1.sh
```

The PostgreSQL 17 gate proves exact recursive role inheritance, transaction
rollback on Sandbox mismatch and the atomic four-projection terminal chain.
The G21.1 gate also reruns G21.0 and requires the current host to remain
`ISOLATION_UNAVAILABLE`; it is not production promotion evidence.

## 21. G21.2 read-only Broker and Artifact canary

The strict plan and activation schemas are
`schemas/neo-agent-broker-artifact-canary-plan.schema.json` and
`schemas/neo-agent-broker-artifact-canary-activation.schema.json`. The plan is
synthetic and admits exactly five actions: Project read, Workspace read, one
exact auth-none MCP read, Artifact publication and a possible-send read. Every
Sandbox is rootless, read-only, capability-free and `networkMode=none`; the plan
contains no Secret, Provider, database, object-store, MCP token or relay key.

Runner ingress has three disjoint caller policies:

```text
spiffe://neo-chat/agent-runtime-control
  -> probe, list, reconcile
spiffe://neo-chat/agent-runtime-root-canary
  -> probe, list, reconcile, launch, heartbeat, cancel
spiffe://neo-chat/agent-runtime-broker-canary
  -> probe, list, reconcile, launch, heartbeat, cancel, prepare, commit
```

`neo-runnerd` forwards Prepare/Commit only to the private literal-HTTPS mTLS
relay. The relay transport identity is exactly
`spiffe://neo-chat/neo-runner-broker-relay`, distinct from all callers. It
accepts no lifecycle method, re-verifies the original authority ticket and
unsigned body fingerprint, and resolves the exact user, Grant and Registry
from the mounted plan before invoking `agentbroker.Service`. Network or relay
ambiguity never creates local authority.

Registered read executors require a frozen read-classified, idempotent,
automatic Registry entry. File reads rewalk the exact immutable root and reject
traversal, links, special files and byte overflow. MCP admits only the pinned
manifest server and exact read Tool. Receipts contain canonical result
fingerprints only. Prepare and Commit exact replay do not execute twice; stale
generation/lease fails before executor or object access. A possible send whose
status cannot be proven becomes terminal `outcome_unknown` and replay never
dispatches again.

Migration `091_agent_artifact_publication` owns function-only Artifact
authority. `agent_artifact_control` has no direct `agent_artifacts` DML, schema
CREATE or owner membership. The dedicated LOGIN recursively inherits exactly
`agent_orchestrator_runtime,agent_runner_control,agent_effect_control,
agent_artifact_control`. Authorize and attach both recheck intent, Tool/action,
user, Run, Attempt generation/live lease, snapshot, Grant/Registry,
non-revocation, Kill Switch, byte bound and media allowlist. Publication scans
one bounded byte snapshot, writes the object before the row, deletes the object
on row failure and cleans quarantine on success or rejection. Exact row replay
is idempotent; Artifact ID/name/object-key collisions fail closed.

The `broker_artifact_canary` activation record binds migration head `095`, the
exact release/target/policy, Runner and relay endpoints, both mTLS trust tuples,
authority key, plan and zero Artifact/quarantine residue. The Compose profile
is independent and default-off. The service may hold its narrow database,
object-store and MCP credentials; Runner and Sandbox may not. No API/Chat route,
mutable Project action, generic MCP, credentialed Tool, Provider, Secret,
Egress, Child, Cron or Learning path is enabled.

Required focused gates are:

```bash
bash scripts/verify-agent-broker-canary-activation.sh
bash scripts/verify-agent-broker-canary-preflight.sh
bash scripts/verify-agent-artifact-publication-postgres17.sh
bash scripts/verify-agent-runtime-g21-2.sh
```

The source, Compose and disposable PostgreSQL proofs are not live evidence. The
current host must still return `ISOLATION_UNAVAILABLE`.

## 22. G21.3 offline-approved synthetic Project mutation canary

The strict schemas are
`schemas/neo-agent-project-mutation-canary-plan.schema.json`,
`schemas/neo-agent-project-mutation-approval.schema.json` and
`schemas/neo-agent-project-mutation-canary-activation.schema.json`. The plan is
synthetic and admits one action only:

```text
project.patch / project.write / apply_patch
classification=mutable; idempotent=false; approval=per_commit
resource=one exact project-canary/* identifier; one flat UTF-8 path; no delete
```

Runner ingress has four disjoint caller policies. G21.0-G21.2 method sets are
unchanged; the fourth caller receives the same reviewed lifecycle plus
Prepare/Commit set as the Broker canary but routes through a different relay:

```text
spiffe://neo-chat/agent-runtime-control
  -> probe, list, reconcile
spiffe://neo-chat/agent-runtime-root-canary
  -> probe, list, reconcile, launch, heartbeat, cancel
spiffe://neo-chat/agent-runtime-broker-canary
  -> probe, list, reconcile, launch, heartbeat, cancel, prepare, commit
spiffe://neo-chat/agent-runtime-project-canary
  -> probe, list, reconcile, launch, heartbeat, cancel, prepare, commit
```

`neo-runnerd` selects the Project relay only for the authenticated Project
caller. Its outbound identity is exactly
`spiffe://neo-chat/neo-runner-project-relay`; it must differ from the Broker
relay identity and endpoint. The relay re-verifies the original signed
authority ticket, unsigned request fingerprint, caller, Runner, Attempt and
plan before invoking Broker. A partial tuple, hostname, public/wildcard/
link-local endpoint, identity reuse or cross-route fails before forwarding.

The operator signs one short-lived
`neo.agent-project-mutation-approval/v1` document with a dedicated Ed25519 key
that never enters the process or Git. The canary mounts only the public key and
signed document. The document binds release commit, current migration head
`092`, target, stable activation binding fingerprint, plan, caller, exact
request and idempotency identities, Tool/action/resource/base/path/content,
actor, reason and validity window. The activation record binds the approval
document and public-key fingerprints. Runner authority, approval and TLS keys
must be distinct.

The stable activation binding is domain-separated over stage, release, target,
Runner, plan, caller and relay endpoint. Do not bind approval to the raw
activation record SHA: the activation record also binds the approval document,
which would create a hash cycle. Do not pre-sign random Broker Intent or Commit
IDs either. Prepare first establishes the exact immutable intent; verified
approval is then appended under one fixed approval ID. The durable approval and
intent collision fences make any second intent/action fail closed.

Migration `092_agent_project_mutation_canary` is not a user Project store. An
operator may provision only the reviewed synthetic baseline while every canary
is off. Runtime receives function-only
`agent_project_mutation_control`; the ninth LOGIN recursively inherits exactly
`agent_orchestrator_runtime,agent_runner_control,agent_effect_control,
agent_project_mutation_control`, with no owner membership, schema CREATE,
provision EXECUTE or direct table DML.

The CAS function rechecks the exact committing intent, approved `per_commit`
fact, user/Project, Run/Step/Attempt, generation and live lease, snapshot,
Grant/Registry, non-revocation, Kill Switch epoch/mode, resource, base
revision, flat path, UTF-8 content, byte budget and content/mutation
fingerprints. Resource update and immutable receipt append are atomic. Exact
replay returns the same receipt; idempotency key, intent, resource, base,
content or mutation collisions reject. Status has exactly three meanings:

- matching receipt: `committed`;
- no receipt plus clean unchanged base: `rejected` (provably not sent);
- any other resource/receipt state: `outcome_unknown`.

The Broker never redispatches after a possible send. Acknowledgement loss after
CAS resolves from the durable receipt. Status unavailable or conflicting state
terminalizes Run, Step and Attempt as `outcome_unknown`. Cleanup runs only
after the Broker committed fact, restores the exact baseline, is replay-safe,
and retains content-free receipt and cleanup facts. Restart reconciliation may
perform cleanup but may not issue a second CAS.

Required focused gates are:

```bash
bash scripts/verify-agent-project-canary-activation.sh
bash scripts/verify-agent-project-canary-preflight.sh
bash scripts/verify-agent-project-mutation-postgres17.sh
bash scripts/verify-agent-runtime-g21-3.sh
```

The profile and `AGENT_PROJECT_MUTATION_CANARY_ENABLED` remain default off.
G21.0-G21.2 must already be ready while all broad Runtime/Broker mutation, MCP
write, Egress, Secret, Child, Cron, Skill-install and Learning switches remain
false. Runner and Sandbox receive no Project database URL, approval material,
authority private key, relay server key, S3/MCP/Provider/vault credential or
generic network. No API/Chat path or user Project is enabled. The current host
still returns `ISOLATION_UNAVAILABLE`.

## 23. G21.4 synthetic depth-one Child canary

The strict schemas are
`schemas/neo-agent-child-run-canary-plan.schema.json` and
`schemas/neo-agent-child-run-canary-activation.schema.json`. The plan contains
exactly one synthetic Parent and one Child. It freezes the user, subject,
model, Package/Runtime fingerprints, idempotency identities, Sandboxes, argv,
leases, grant window and four-dimensional budgets. The Child is a strict
budget/expiry subset and neither Sandbox has network, capability or credential
authority.

Runner ingress adds one lifecycle-only policy:

```text
spiffe://neo-chat/agent-runtime-child-canary
  -> probe, list, reconcile, launch, heartbeat, cancel
  -> no prepare, commit or relay
```

The Parent Grant and Registry contain exactly `delegate_task/create` for
`g21.4/synthetic-child`. The Child requests that identity as an explicit
negative proof, but server derivation produces a capability-empty Grant and
physically empty depth-one Registry before fingerprinting. Alias/capability
rebinding, a second Child, depth two and every subject/model/package/runtime/
Grant/Registry/expiry/budget widening fail before Runner launch.

Migration `093_agent_child_canary_reap_transport` retains migration `087` and
adds two function-only seams. `agent_delegation_reap_inventory(limit)` returns
the exact Child Step/Attempt/generation, optional Runner projection and latest
matching launch-authority expiry without lease tokens.
`agent_delegation_complete_reap(...)` keeps failures retryable; success rejects
an active launch authority or mismatched Sandbox and atomically terminalizes
the exact Runner projection plus durable reap. Exact success replay is stable.
Dirty down fails with `AGENT_CHILD_REAP_TRANSPORT_DOWN_REQUIRES_CLEAN`; clean
down restores the migration-087 completion function.

The controller always reconciles stale Parents and failed reaps before enqueue.
Existing lineage is authoritative, so restart never selects another Child
idempotency key. A live tokenless Attempt waits for expiry. A queued Child may
launch only from its exact live Parent Attempt; otherwise Child-first cascade,
Runner absence proof and durable reap complete before Parent cleanup. Parent
reacquisition is cleanup-only after its sole Child is terminal. Health requires
the Parent and Child canceled, zero pending reaps, and no matching Runner
Sandbox.

Required focused gates are:

```bash
bash scripts/verify-agent-child-canary-activation.sh
bash scripts/verify-agent-child-canary-preflight.sh
bash scripts/verify-agent-child-canary-postgres17.sh
bash scripts/verify-agent-runtime-g21-4.sh
```

The profile, `AGENT_CHILD_CANARY_ENABLED` and `AGENT_DELEGATION_ENABLED` remain
default off. G21.0-G21.3 must be ready while generic Runtime, Broker mutation,
MCP write, Egress, Secret, Scheduler, Skill-install and Learning flags remain
false. No API/Chat or package-selected delegation path is enabled. Checked-in
activation is template-only and this host remains `ISOLATION_UNAVAILABLE`.

## 24. G21.5 exact Cron and Draft-learning workers

The strict schemas are
`schemas/neo-agent-cron-worker-{plan,activation}.schema.json` and
`schemas/neo-agent-draft-learning-worker-{plan,activation}.schema.json`.
Both activation stages require current G21.0-G21.4 prerequisite evidence,
migration head `095`, an exact plan SHA-256 and fresh production evidence.
`cron_worker` and `draft_learning_worker` are independently default off; one
flag or profile never implies the other or any broad Runtime, Scheduler,
Learning, Skill-install, Broker or product-cohort flag.

Migration `094_agent_cron_learning_activation` stores immutable exact targets.
The `agent_cron_worker` role may call only activation-scoped target read, due
claim, cursor advance, trigger claim/enqueue/release, reconcile and prune
functions. Target membership is part of each locking candidate query. Revision,
fingerprint, lifecycle or activation-window drift rejects before claim. The
role cannot create/edit Templates, provision/disable targets, read tables or
claim another Template.

The `agent_learning_worker` role may call only activation-scoped Draft read,
check claim/complete/release, Draft-only Runner attempt/request/result,
cleanup, reconcile and prune functions. It cannot call generic Claim, Propose,
diff/review, Reject, Promote, candidate insertion or package-version insertion.
The target binds the Draft/package/runtime/archive, Runner snapshot, Workspace,
isolation/evaluation suites, plan and validity window. Human Promote remains an
administrator-only migration-089 transaction and creates a new immutable
candidate/package version.

Draft Runner attempts are not Agent Runs or Agent Attempts and confer no Broker,
Provider, Cron, delegation or admission authority. The sixth caller policy is:

```text
spiffe://neo-chat/agent-runtime-draft-learning
  -> probe, list, reconcile, launch, result, cancel
  -> no heartbeat, prepare, commit or relay
```

`result` is a read-only RPC for the exact authenticated Attempt and fixed
artifact name. The strict artifact is at most 64 KiB and binds activation,
Draft/fingerprint, check generation/kind, package/runtime/archive, Workspace,
suite, status, reason, duration and bounded integer metrics. SQL accepts a
receipt only after exact result persistence and successful Sandbox cancel/reap.
Authority and response replay are request/nonce/fingerprint fenced; lost tokens
wait for lease/authority expiry and reconciliation before a new Draft claim
generation.

Required gates are:

```bash
bash scripts/verify-agent-cron-worker.sh
bash scripts/verify-agent-cron-worker-postgres17.sh
bash scripts/verify-agent-draft-learning-worker.sh
bash scripts/verify-agent-draft-learning-worker-postgres17.sh
bash scripts/verify-agent-runtime-g21-5-preflight.sh
bash scripts/verify-agent-runtime-g21-5.sh
```

The PostgreSQL gates prove two due Templates and two quarantined Drafts, exact
claim isolation, real function-only LOGIN denial, Runner result replay,
human-decision separation, scoped cleanup, dump/restore and guarded clean
down/up. Production rollback disables one target/profile and retains migration
`094`; destructive down is disposable-only after every target, claim, Runner
attempt, cleanup and LOGIN membership guard is clean. No API/Chat path or user
cohort is enabled, and this development host remains
`ISOLATION_UNAVAILABLE`.

## 25. G21.6 bounded product canary and final promotion

Migration `095_agent_product_canary_activation` is the only product-canary
request authority. `go_api_runtime` may call current-user status and enqueue
functions only. The strict HTTP body contains exactly
`expectedPolicyRevision` and `expectedGeneration`; the server supplies the
request ID and fingerprint. Prompt, arguments, Package/model selection, argv,
Tool Registry, Workspace, Egress, Secrets and resource overrides are rejected
before persistence. PostgreSQL independently re-derives the canonical request
fingerprint and transaction-locks the same user/policy/generation so concurrent
submissions converge on the first immutable request instead of a unique-key
failure.

An operator-provisioned activation binds the exact release, migration head
`095`, Shadow policy, admitted Package/Runtime, fixed plan and seven distinct
G21.0-G21.5 evidence fingerprints. It is valid for at most 24 hours and 20
requests, cannot widen in place, and immediately blocks new requests when
disabled, expired or drifted. With no activation, Shadow remains
`ISOLATION_UNAVAILABLE`; with a current activation and every existing fence,
the status may return `PRODUCT_CANARY_READY` and a finite remaining budget.

The dedicated worker LOGIN inherits exactly `agent_product_canary_worker`,
`agent_orchestrator_runtime` and `agent_runner_control`. Product functions use
activation-scoped lease generations, bounded retries and replay-safe completion.
Claim accepts only `queued`; an expired claim must first pass through reconcile
and consume one failure attempt before it may be queued again.
The fixed worker plan has depth zero, an empty Tool Registry, read-only rootfs,
no network/Egress/Secrets, UID/GID `10001`, 30-second wall/lease bounds and
fixed argv `/opt/neo/bin/product-canary --bounded-smoke`. The Runner caller is:

```text
spiffe://neo-chat/agent-runtime-product-canary
  -> probe, list, reconcile, launch, heartbeat, cancel
  -> no result, prepare, commit or relay
```

Completion re-derives the canonical receipt fingerprint and requires an
unexpired request claim, the exact request-derived product idempotency key,
ordinal-zero `product_canary` Step and terminal Run/Attempt. It appends one
immutable content-free receipt. Worker health reads only expired-claim and
pending-terminalization counts, then uses Runner Probe/List to require zero
Sandbox residue; it never reconciles state. Only the operator-owned promotion
function may append a
`PROMOTION_READY` record binding the same activation, request, receipt, release
and closure fingerprint. API and workers have neither activation provisioning
nor promotion authority. Dirty migration down is rejected while any activation,
request, receipt or promotion fact remains.

Required gates are:

```bash
bash scripts/verify-agent-product-canary-activation.sh
bash scripts/verify-agent-product-canary-postgres17.sh
bash scripts/verify-agent-runtime-g21-6-preflight.sh
bash scripts/verify-agent-production-closure.sh
bash scripts/verify-agent-runtime-g21-6.sh
```

Checked-in evidence is template/offline-only. The development host remains
`ISOLATION_UNAVAILABLE`; disposable PostgreSQL or schema validation cannot
create live activation or final-promotion evidence.

## 26. Current local Skill execution

The single-server product uses the separate Hermes-style `local_direct`
contract in [`local-skill-runtime.md`](./local-skill-runtime.md). Installed
Skills need only `SKILL.md` and may include `scripts/`, `references/`, and
`assets/`; they do not need `neo.runtime.json` or an OCI image.

This current path executes through ordinary Chat native Tools as the Backend
UID/GID in the configured workspace. It requires no Podman, WSL systemd,
`sudo`, restart, Runner mTLS, production activation evidence, or per-Skill
container. Its command guards and budgets reduce accidental damage but do not
create an isolation boundary.

The G20/G21 durable OCI code and evidence below remain disabled optional
history. Their `ISOLATION_UNAVAILABLE` state does not gate `local_direct` Skill
discovery or Chat execution.

## 27. Phase 0 verification

Run:

```bash
bash scripts/verify-agent-runtime-phase0.sh
```

It validates schemas, positive/negative fixtures, cross-contract invariants,
required design anchors and the current fail-closed code execution route. It is
offline and must never claim the production Runner or Isolation Acceptance Suite
passed.

The implemented G20.4-G20.10 source/control, product, cutover and operations
foundations additionally require:

```bash
bash scripts/verify-agent-broker.sh
bash scripts/verify-agent-broker-postgres17.sh
bash scripts/verify-agent-delegation.sh
bash scripts/verify-agent-delegation-postgres17.sh
bash scripts/verify-agent-cron.sh
bash scripts/verify-agent-cron-postgres17.sh
bash scripts/verify-agent-learning.sh
bash scripts/verify-agent-learning-postgres17.sh
bash scripts/verify-agent-product-shadow.sh
bash scripts/verify-agent-product-shadow-postgres17.sh
bash scripts/verify-agent-legacy-cutover.sh
bash scripts/verify-agent-legacy-cutover-postgres17.sh
bash scripts/verify-agent-production-closure.sh
bash scripts/verify-agent-runtime-g21-0.sh
bash scripts/verify-agent-root-canary-activation.sh
bash scripts/verify-agent-root-canary-postgres17.sh
bash scripts/verify-agent-runtime-g21-1.sh
bash scripts/verify-agent-broker-canary-activation.sh
bash scripts/verify-agent-broker-canary-preflight.sh
bash scripts/verify-agent-artifact-publication-postgres17.sh
bash scripts/verify-agent-runtime-g21-2.sh
bash scripts/verify-agent-project-canary-activation.sh
bash scripts/verify-agent-project-canary-preflight.sh
bash scripts/verify-agent-project-mutation-postgres17.sh
bash scripts/verify-agent-runtime-g21-3.sh
bash scripts/verify-agent-child-canary-activation.sh
bash scripts/verify-agent-child-canary-preflight.sh
bash scripts/verify-agent-child-canary-postgres17.sh
bash scripts/verify-agent-runtime-g21-4.sh
bash scripts/verify-agent-cron-worker.sh
bash scripts/verify-agent-cron-worker-postgres17.sh
bash scripts/verify-agent-draft-learning-worker.sh
bash scripts/verify-agent-draft-learning-worker-postgres17.sh
bash scripts/verify-agent-runtime-g21-5-preflight.sh
bash scripts/verify-agent-runtime-g21-5.sh
bash scripts/verify-agent-runner.sh
bash scripts/verify-agent-runtime-phase0.sh
bash scripts/verify-agent-runner-host.sh # expected nonzero on the current host
```

The host command must still report `ISOLATION_UNAVAILABLE` here. Passing the
offline and disposable-database gates closes source/control contracts only; it
does not authorize Agent execution, Chat invocation, a production relay,
Scheduler, generic Project mutation or user Projects. The one synthetic G21.3
CAS remains independently default off. Production additionally needs a fresh
exact-host record evaluated as `PROMOTION_READY`.
