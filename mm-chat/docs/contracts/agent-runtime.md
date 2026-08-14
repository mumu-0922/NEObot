# Neo Agent Runtime Executable Contract

Status: G20.1 supply-chain, G20.2 durable Orchestrator, G20.3 Runner, G20.4
brokered effects, G20.5 depth-1 Child delegation and G20.6 durable Cron
scheduling source/control foundations are implemented. Exact-host isolation and
production Runner/Broker/Child/Scheduler promotion are held; all production
Agent execution remains disabled.

## 1. Scope and hard gates

This contract defines the stable boundary among Go Durable Orchestrator,
PostgreSQL, non-root `neo-runnerd`, per-Run rootless OCI Sandbox and mediated
Tool/Egress/Secret/Workspace/Artifact services.

In scope:

- Agent Skills package plus Neo Runtime Manifest admission;
- immutable Skill/runtime bundle, fingerprint and SBOM authority;
- durable Run/Step/Attempt state, lease, recovery and events;
- Capability Grant, Egress policy, Secret Broker and budgets;
- Runner capability probe, launch, heartbeat, cancel/kill and side effects;
- one-level Child Agents, Cron snapshots and Draft-only learning;
- legacy text-Skill cutover and rollback requirements.

Hard gate: no executor, route or feature flag may make a production Agent Run
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

Learning output is an untrusted Draft stored in quarantine with source Run IDs,
evidence, tests and proposed package files. Automated evaluation cannot set
admitted/installable status. Human Promote reruns admission and produces a new
fingerprint; rejection/deletion cannot affect the source Run or installed Skill.

## 13. Kill Switch contract

Kill Switches are durable denies with actor, scope, mode, reason, revision and
timestamps. Broader deny overrides narrower state. Modes:

- `deny_new`: no new admission, Run, lease or action in scope;
- `cancel`: fence new mutable actions and request cooperative stop;
- `kill`: fence leases immediately and kill exact Sandbox cgroup/process group.

Every admission, lease, heartbeat, Broker action, Prepare and Commit checks the
current switch epoch. Runtime disabled or Runner unavailable never stops
retention, artifact cleanup, expired-intent cleanup, orphan reconciliation or
audit access.

## 14. Error matrix

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

No error includes secret, raw arguments/result, package contents, Workspace
paths/content, host paths or provider payloads.

## 15. Isolation Acceptance Suite

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

## 16. Legacy text-Skill cutover

The future cutover deletes, rather than migrates or wraps:

```text
installedSkills / customSkills / activeSkillIds / skillAutoSelect
Conversation.config.activeSkills / Workspace.activeSkills
browser Skill selection and system-context assembly
legacy catalog/custom Skill definitions and execution references
```

Historical message content is preserved, but old `skillInvocations` render only
the read-only fact “旧版技能已退役”; they cannot reopen or execute old definitions.
Phase 0 deliberately changes none of this state. Final cutover requires backup,
inventory counts/hashes, storage-version purge tests, server/runtime promotion,
history projection proof and an all-or-nothing rollback window.

## 17. Phase 0 verification

Run:

```bash
bash scripts/verify-agent-runtime-phase0.sh
```

It validates schemas, positive/negative fixtures, cross-contract invariants,
required design anchors and the current fail-closed code execution route. It is
offline and must never claim the production Runner or Isolation Acceptance Suite
passed.

The implemented G20.4/G20.5/G20.6 source/control foundations additionally require:

```bash
bash scripts/verify-agent-broker.sh
bash scripts/verify-agent-broker-postgres17.sh
bash scripts/verify-agent-delegation.sh
bash scripts/verify-agent-delegation-postgres17.sh
bash scripts/verify-agent-cron.sh
bash scripts/verify-agent-cron-postgres17.sh
bash scripts/verify-agent-runner.sh
bash scripts/verify-agent-runtime-phase0.sh
bash scripts/verify-agent-runner-host.sh # expected nonzero on the current host
```

The host command must still report `ISOLATION_UNAVAILABLE`. Passing the offline
and disposable-database gates does not authorize an Agent API, Chat/frontend
path, production relay, Scheduler, Project mutation, mutable canary or
text-Skill deletion.
