# Agent Runtime Operations Contract

## Scenario: Deploy and operate `neo-runnerd` with rootless OCI

### 1. Scope / Trigger

Apply this contract when changing Agent Runtime host prerequisites, Runner
service/account/mTLS, OCI images, Workspace/Scratch/Artifact boundaries,
capability probe, isolation tests, Kill Switches, release, backup/restore or
legacy Skill cutover. Protect `.env.single-server`, `data/`, `secrets/` and
`backup/`.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-runtime-phase0.sh
bash mm-chat/scripts/verify-agent-orchestrator.sh
bash mm-chat/scripts/verify-agent-orchestrator-postgres17.sh
bash mm-chat/scripts/verify-agent-runner.sh
bash mm-chat/scripts/verify-agent-runner-postgres17.sh
bash mm-chat/scripts/verify-agent-broker.sh
bash mm-chat/scripts/verify-agent-broker-postgres17.sh
bash mm-chat/scripts/verify-agent-delegation.sh
bash mm-chat/scripts/verify-agent-delegation-postgres17.sh
bash mm-chat/scripts/verify-agent-product-shadow.sh
bash mm-chat/scripts/verify-agent-product-shadow-postgres17.sh
bash mm-chat/scripts/verify-agent-legacy-cutover.sh
bash mm-chat/scripts/verify-agent-legacy-cutover-postgres17.sh
bash mm-chat/scripts/verify-agent-runner-host.sh # expected nonzero until exact host is prepared
```

The Runner, Broker and delegation gates prove source/control and disposable
PostgreSQL behavior. The host gate must return `ISOLATION_UNAVAILABLE` here and
becomes promotion evidence only when the exact approved service account/release
passes the full suite.

### 3. Contracts

- `neo-runnerd` is a dedicated host-side non-root service, separate from
  `mcp-runner`, and has no browser bearer, database, Redis, object-store,
  Provider vault or Docker/Podman socket credential.
- Every Run gets a fresh rootless OCI Sandbox: nonzero UID/GID, read-only root,
  empty capabilities, `no_new_privs`, reviewed seccomp, cgroup v2 CPU/memory/
  PID/time/output limits, no host PID/IPC/network/project bind/device/socket.
- The exact Runner service account must pass userns/subuid/subgid, cgroup v2,
  seccomp, storage/network and kill/reap capability probes. Docker daemon health
  is not proof; rootful/privileged/sudo fallback is forbidden.
- Runner RPC is private mTLS with version, deadline, body/concurrency bounds,
  nonce/request replay fence and exact lease/snapshot fingerprints.
- Project Workspace is a fully rehashed read-only snapshot. Scratch is a
  size-bounded tmpfs and host broker staging is always destroyed. Artifact uses
  a per-Attempt framed Unix intake into Runner quarantine. Sandbox never
  receives object-store credentials or arbitrary host mounts.
- Egress is `none`, exact allowlist or Tool-specific broker. Re-resolve/recheck
  DNS, redirects and reconnects; deny localhost/private/link-local/metadata/raw
  IP forms and origin-changing credential forwarding.
- Secret Broker injects no durable/env/prompt value; use action-scoped short-TTL
  handles or Broker-side credential application and prove zero leakage.
- Runtime disabled still runs retention, cleanup, expired-intent removal and
  orphan reconciliation.
- Migration `084` supplies database-only Kill Switch fencing, recovery,
  projection rebuild and retention before any Runner exists. A passing G20.2
  drill is not rootless isolation evidence and must not enable Runtime.
- Migration `085` supplies Runner/lease-owner-bound replay and expected Sandbox
  lifecycle authority through `agent_runner_control`; the host daemon has no DB
  role. Cleanup/reconcile and completed replay pruning remain usable while
  execution is disabled.
- Migration `086` supplies durable Prepare/approval/Commit/receipt and
  secret-handle-digest authority through `agent_effect_control`. The Runner,
  public API and Orchestrator roles gain no table DML. Its default production
  relay remains unavailable; do not manually invoke database functions as an
  executor or promote the deterministic Project CAS fake.
- Migration `087` supplies held root/child lineage, Parent reservations,
  launch admission, settlements and durable reap work through
  `agent_delegation_control`. API, Orchestrator, Runner and effect roles gain no
  delegation DML; `neo-runnerd` remains credential-free. No public/startup Child
  worker exists, and operators must not manufacture root authority or Child Runs
  in a live database.
- Parent cancel/kill and reconcile fence Child leases plus terminal
  Attempt/Step/Run state before host reaping. Reap failure remains durable and
  retryable; never restore a lease or delete delegation facts to clear health.
- A possibly sent mutable effect must be reconciled by its exact stable status
  key. Without exact committed/not-sent proof, record `outcome_unknown`; never
  retry with another key. Runtime-off operation still expires intents/revokes
  handles and permits receipt reconciliation and cleanup.
- Pre-Commit cancellation uses the migration `086` append-only cancellation
  authority and races under the same exact intent lock as Commit. A cancellation
  winner terminalizes the prepared Attempt and performs zero executor calls; a
  `committing`/terminal intent rejects cancellation and follows receipt or
  `outcome_unknown` recovery instead.
- Grant revocation remains append-only. The control service must pair its
  durable revocation with immediate zeroization of matching memory-only Secret
  bytes; the database revokes every active durable handle digest.
- G20.9 legacy source cutover is hard deletion. Live apply still requires the
  verified G20.8 browser backup, a full database backup/count, clean restart and
  rollback rehearsal. Never mix dual execution.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| rootless runtime/feature/version missing | Runner not ready; no rootful fallback |
| mTLS/version/replay failure | reject without launch or secret output |
| OCI config requests root/capability/host mount/network | reject before create |
| stale/unknown Sandbox after restart | kill exact cgroup and reconcile |
| cgroup/output/time budget exceeded | terminate exact Sandbox; Runner stays healthy |
| Kill Switch `kill` | fence lease/effects, kill descendants, remove Scratch |
| restore sees pre-restore live Sandbox | kill; never trust old lease/nonces |
| cleanup object removal fails | retain durable queue/state and retry |
| Child reap fails after Parent cascade | keep Child terminal/lease-fenced; retry exact durable reap |
| migration `087` passes but exact host is held | keep production Child execution disabled |

### 5. Good / Base / Bad Cases

- **Good**: pinned Runner/runtime on a prepared host passes the exact Isolation
  Acceptance Suite, then a no-network/no-secret read-only canary runs.
- **Base**: Phase 0 verifier passes while no Runner/runtime is installed or
  enabled; output explicitly says isolation acceptance was not executed.
- **Bad**: `docker run --privileged`, mounting `/var/run/docker.sock` or source,
  adding `CAP_SYS_ADMIN`, host networking, secrets in env, or calling a healthy
  container proof of isolation.

### 6. Tests Required

- Capability probe and OCI config inspection as exact Runner account.
- Filesystem/mount/socket/device/procfs/symlink/hardlink/path-race negatives.
- DNS rebinding/redirect/reconnect/metadata/private-IP/credential-leak negatives.
- Secret canary across env/argv/proc/prompt/output/files/artifacts/logs/metrics.
- CPU/memory/PID/disk/output/wall exhaustion and descendant/orphan/reboot reap.
- Lease reclaim, Prepare/Commit crash matrix, depth-1 Registry and Runtime-off
  cleanup proof.
- Migration `087` concurrent reservation, stale Parent/Child launch, terminal
  settlement, cascade/reap failure, terminal/expired/reclaimed/Kill-Switch
  recovery, least privilege, dump/restore and clean down/up; all older tail
  drills must peel the empty product tail and return to current head `090`.
- Paired PostgreSQL/object backup, restore-with-Runtime-off and reconciliation.
- G20.1 backup/restore pairs migration `083` rows with all three immutable
  object prefixes: `skill-quarantine/`, `skill-packages/`, and `skill-sboms/`.
  The temporary MinIO drill exports PostgreSQL coordinates and `mc stat`s every
  sampled key before cleanup.

### 7. Wrong vs Correct

#### Wrong

```text
Docker works -> start a privileged container -> call Sandbox complete
```

#### Correct

```text
exact non-root account probe -> immutable launch envelope -> rootless OCI
-> broker-only I/O -> fingerprint-bound acceptance evidence -> bounded canary
```

## Scenario: Operate the held Cron control plane

### 1. Scope / Trigger

Apply this contract when deploying migration `088`, assigning Cron database
roles, running Cron verification, reconciling expired scheduling claims,
pruning Cron history or rolling the migration back. This scenario does not
authorize a production Scheduler service.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-cron.sh
bash mm-chat/scripts/verify-agent-cron-postgres17.sh
```

Migration `088_agent_cron_foundation` and `agent_cron_control` are the durable
operational boundary. The exact-host gate remains expected-nonzero
`ISOLATION_UNAVAILABLE` until the separate Runner promotion criteria pass.

### 3. Contracts

- Install migration `088` with a NOLOGIN owner and the dedicated
  `agent_cron_control` role. That control role has SELECT plus exact function
  execution and no table DML or membership in the owner role; API,
  Orchestrator, Runner, Broker and delegation roles gain no Cron authority.
- Keep Runtime and Scheduler disabled. Do not add an HTTP/Chat/frontend/startup
  route, Redis wake loop, Compose worker or production execution wiring merely
  because the source and PostgreSQL gates pass.
- Reconciliation may clear expired cursor claims and return expired trigger
  claims to pending. Cleanup must use bounded `agent_cron_prune` batches and
  retain live/enqueued Run facts until eligible.
- Pause/resume/delete through controlled lifecycle functions. Resume advances to
  the next future instant and records only sanitized missed-window facts; never
  manufacture a backfill directly in tables.
- Keep `.env.single-server`, `data/`, `secrets/` and `backup/` untouched. Cron
  rows and audits must not contain prompts, Secret values, Workspace bytes,
  Tool arguments/results or credentials.
- The down migration is deliberately guarded by
  `AGENT_CRON_DOWN_DATA_EXISTS`. Drain/tombstone and boundedly prune eligible
  state before an intentional rollback; never delete protected runtime state or
  bypass the guard.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| production Scheduler/Runtime switch remains off | no schedule worker starts; reconcile/prune remain available |
| stale/expired claim worker attempts mutation | `STALE_CLAIM`; operator reconciles rather than forcing table DML |
| trigger loses current authority | sanitized Cron denial fact; no Run enqueue |
| `agent_cron_control` obtains table DML or another runtime role obtains Cron access | deployment gate fails; do not promote |
| down attempted while Cron data/audit remains | `AGENT_CRON_DOWN_DATA_EXISTS`; rollback stops intact |
| source/PG17 gates pass but exact host is unprepared | keep production disabled; host result remains `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good**: deploy `088`, prove least privilege and restart/idempotency on a
  disposable PostgreSQL 17 database, then leave production scheduling disabled.
- **Base**: operators run bounded reconcile/prune while Runtime is off; retained
  Cron and Orchestrator history remains auditable.
- **Bad**: grant direct Cron table UPDATE, manually relink an occurrence, delete
  audit rows to force down, or treat a disposable-database pass as Scheduler or
  exact-host promotion evidence.

### 6. Tests Required

- Run both Cron signatures plus Phase 0, focused race/vet tests and
  `go mod verify`.
- PostgreSQL 17 must prove fresh/replay, concurrency, restart reclaim,
  acknowledgement-loss idempotency, bounded retry/overlap, all authority
  denials, least privilege, content-free dump/restore, guarded down and clean
  down/up.
- Every prior PostgreSQL tail drill must peel `090`, `089` and then `088` before
  testing its older guard and finish reapplied at head `090`.
- The full standalone gate must pass, while the exact-host Runner gate remains
  expected-nonzero `ISOLATION_UNAVAILABLE` unless a separately approved host
  promotion is in scope.

### 7. Wrong vs Correct

#### Wrong

```text
PG17 Cron drill passed -> add startup Scheduler -> enable production triggers
```

#### Correct

```text
deploy guarded migration 088 -> prove narrow role + durable replay/recovery
-> keep Scheduler/Runtime disabled -> promote only in a later explicit gate
```

## Scenario: Operate the held Draft-learning control plane

### 1. Scope / Trigger

Apply this contract when deploying migration `089`, assigning Agent Learning
database roles, running Draft-learning gates, reconciling claims, deleting
quarantine objects, pruning history, backing up/restoring learning authority or
rolling the migration back. This scenario does not authorize a public Draft
surface, startup worker, evaluator, Sandbox execution or production Promote.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-learning.sh
bash mm-chat/scripts/verify-agent-learning-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-phase0.sh
bash mm-chat/scripts/verify-agent-runner-host.sh # expected nonzero here
```

Migration `089_agent_draft_learning` plus `agent_learning_owner` and
`agent_learning_control` are the durable operational boundary.

### 3. Contracts

- Install `089` with NOLOGIN owner/control roles. Control has SELECT plus exact
  function execution and no table DML or owner membership; API, Orchestrator,
  Runner, Broker, delegation and Cron roles gain no learning authority.
- Keep `AGENT_LEARNING_ENABLED=false` and `AGENT_RUNTIME_ENABLED=false`. G20.7
  adds no environment/Compose/startup wiring; do not create it merely because
  source or disposable PostgreSQL gates pass.
- Preserve PostgreSQL plus all `skill-drafts/`, `skill-quarantine/`,
  `skill-packages/` and `skill-sboms/` objects as one backup authority set.
  Restore with Learning/Runtime off, validate fingerprints, then reconcile
  expired check/cleanup claims before any later product promotion.
- Cleanup deletes the exact quarantine object before completing its fenced row.
  Missing object is idempotent success; other storage failures remain bounded
  retry facts. Do not delete rows/audits to make storage health appear green.
- Expired `checking` or claimed-cleanup work is never claimed directly. Run the
  bounded reconciler first so it increments the failed-attempt counter,
  terminalizes exhaustion and only then exposes a new ready generation.
- Quarantine cleanup does not erase an append-only decision. An exact Promote
  replay is served from the decision/admission link without requiring the
  already deleted Draft object; any input mismatch remains denied.
- Diagnostics contain IDs, fingerprints, state, generations, reason codes,
  bounded metrics and counts only. Never log Draft/Skill/prompt/Run/Tool/
  Workspace content, Secret values, credentials or object-store URLs.
- A mismatched content-addressed object is an incident: stop Promote, retain the
  bytes/evidence, activate the relevant Kill Switch, and repair through a new
  verified object/package path. Never overwrite it manually.
- Down is guarded by `AGENT_LEARNING_DOWN_DATA_EXISTS`. Production rollback
  keeps `089` applied and Learning disabled. Only a disposable, verified-empty
  database may execute clean down/up.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Learning/Runtime flags off | no proposal/check/Promote worker starts; read/reconcile/cleanup/prune remain available |
| another runtime role gains Draft access or Promote execution | deployment gate fails; revoke and do not promote |
| expired check/cleanup claim remains | reconcile to a new generation; stale worker stays fenced |
| quarantine object deletion fails | keep retry row and object authority; do not acknowledge/prune |
| object hash/size/key drift is observed | `DRAFT_OBJECT_DRIFT`; stop promotion and preserve incident evidence |
| down attempted with learning state/candidate | `AGENT_LEARNING_DOWN_DATA_EXISTS`; schema remains intact |
| PG17/Phase 0 pass but exact host is unavailable | production stays disabled; `ISOLATION_UNAVAILABLE` remains expected |

### 5. Good / Base / Bad Cases

- **Good**: deploy `089` default-off, pass least-privilege/fresh/replay/
  dump-restore gates, restore with switches off, and reconcile/clean exact
  objects before any later activation review.
- **Base**: no Draft worker is running; operators inspect content-free counts
  and run bounded cleanup for already rejected/promoted Drafts.
- **Bad**: grant table UPDATE, start a learning worker from an ad-hoc shell,
  auto-promote evaluator output, overwrite a digest key, prune before object
  deletion, or force the guarded down migration.

### 6. Tests Required

- Run the four signatures; the host gate remains expected nonzero unless a
  separate exact-host promotion is explicitly in scope.
- PostgreSQL 17 proves fresh/replay, role isolation, claims/reclaim, source and
  human authority, exact decision replay, immutable live state, cleanup,
  content-free dump/restore, guarded down and clean down/up.
- Verify every older Agent/MCP/Assistant/Skill PostgreSQL drill peels empty
  `090` before its original guard and returns to head `090`.
- Run backend race/vet, Phase 0, module/security/quality/change gates and full
  standalone. Confirm `data/`, `secrets/`, `backup/` and live
  `.env.single-server` remain untouched.

### 7. Wrong vs Correct

#### Wrong

```text
PG17 passed -> enable worker -> evaluator promotes -> overwrite current Skill
```

#### Correct

```text
deploy guarded 089 default-off -> verify roles + paired backup/restore
-> reconcile and object-before-row cleanup -> later explicit UI/shadow/host gate
```

## Scenario: Operate Agent Center and held Shadow authority

### 1. Scope / Trigger

Apply this contract when deploying migration `090`, exposing Agent Center,
operating Shadow policy/opt-in state, downloading Agent Artifacts, backing up or
restoring product authority, or preparing G20.9 legacy deletion. This scenario
does not authorize executable Shadow or production Runtime.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-product-shadow.sh
bash mm-chat/scripts/verify-agent-product-shadow-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-phase0.sh
bash mm-chat/scripts/verify-agent-runner-host.sh # expected nonzero here
```

Migration `090_agent_product_shadow`, `agent_product_owner` and the authenticated
`internal/agentcontrol` facade are the operational boundary.

### 3. Contracts

- Deploy `090` with Runtime, Scheduler, Learning and Shadow execution disabled.
  Agent Center may expose held read/control state; it must not start a worker or
  install a Shadow adapter from the API process.
- `go_api_runtime` receives sanitized views and exact product functions only.
  Deny direct worker table DML, Attempt lease/claim, effect Commit, delegation
  launch, Cron trigger and learning-check authority.
- Keep Artifact object keys and storage credentials server-only. Backup
  PostgreSQL product rows and `agent-artifacts/` objects as one fingerprint-
  bound set; restore with all workers off.
- Shadow policy defaults off. Later enablement requires exact administrator,
  admitted package/runtime fingerprints, bounded cohort/window/budgets and
  explicit user opt-in. The exact-host acceptance must pass separately.
- Global/user/admission/Skill Kill Switch, opt-out, expiry, restart, budget and
  fingerprint drift fence observations. Store no prompt/output/Tool/Secret/
  Artifact body or custom external URL in Shadow authority.
- Reconcile/retention and held status reads remain available while disabled.
  Shadow output never becomes Chat, install, admission or promotion authority.
- G20.8 legacy inventory/backup/dry-run was local and non-destructive. G20.9
  removes that temporary UI together with legacy authority. Continue protecting
  `.env.single-server`, `data/`, `secrets/`, `backup/` and unrelated browser
  domains.
- `090` down is guarded by `AGENT_PRODUCT_DOWN_DATA_EXISTS`. Production rollback
  keeps `090` applied and disables Shadow; only verified-empty disposable state
  may down/up.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| exact host unavailable | Agent Center reports `ISOLATION_UNAVAILABLE`; no executable work |
| non-admin changes Shadow policy | `ADMINISTRATOR_REQUIRED`; no revision |
| stale policy/opt-in/boot/fingerprint | stable conflict/fence; no observation |
| applicable Kill Switch | `KILL_SWITCH_ACTIVE`; no adapter/observation |
| observation/error budget exhausted | `BUDGET_EXCEEDED`; new work fenced |
| Artifact owner mismatch | not found; object store is not opened |
| down with product/Shadow state | `AGENT_PRODUCT_DOWN_DATA_EXISTS`; schema retained |

### 5. Good / Base / Bad Cases

- **Good**: deploy `090` default-off, prove least privilege and paired restore,
  expose held Agent Center, and retain content-free Shadow diagnostics only.
- **Base**: no policy exists, Runtime is held, and G20.9 leaves no legacy
  executor or fallback path.
- **Bad**: call an observation function as a canary worker, expose object keys,
  enable an in-process executor, treat PG17 as host acceptance, or recreate
  legacy state after G20.9.

### 6. Tests Required

- Product/Shadow PG17: fresh/replay, ACLs, cross-user ownership, cancel replay,
  Artifact seam, default-off/cohort/opt-in, Kill Switch, budget, restart,
  generation/fingerprint fences, content-free dump/restore and guarded down/up.
- All older Agent/MCP/Assistant/Skill PostgreSQL drills peel empty `090` before
  their original guards and finish at head `090`.
- Run backend race/vet/test, frontend format/lint/typecheck/test/build, Phase 0
  and full standalone. Exact-host remains expected nonzero unless separately
  approved.
- Confirm protected runtime paths and the live env remain byte-untouched.

### 7. Wrong vs Correct

#### Wrong

```text
PG17 passed -> enable Shadow worker in API -> execute package -> delete legacy
```

#### Correct

```text
deploy guarded 090 default-off -> verify API/ACL/backup/reload/no-delete
-> G20.9 source/data deletion -> keep ISOLATION_UNAVAILABLE
-> later exact-host canary gate
```

## Scenario: Apply the G20.9 legacy Skill data cutover

### 1. Scope / Trigger

Apply when preparing, dry-running, executing, verifying or rolling back the
one-way legacy text-Skill deletion. Protect `.env.single-server`, `data/`,
`secrets/` and `backup/`; this contract does not authorize production Runtime.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-legacy-cutover.sh
bash mm-chat/scripts/verify-agent-legacy-cutover-postgres17.sh
psql "$DATABASE_URL" --file mm-chat/scripts/cutover-legacy-skills.sql
psql "$DATABASE_URL" --variable=cutover_apply=true \
  --variable=expected_count="$EXPECTED_COUNT" \
  --variable=backup_fingerprint="sha256:$BACKUP_SHA256" \
  --file mm-chat/scripts/cutover-legacy-skills.sql
```

The psql variables are operator attestations: apply boolean, locked target
count, and lowercase SHA-256 fingerprint of the matching full database backup.

### 3. Contracts

- Before deploying G20.9, use the still-running G20.8 release to freeze edits
  and capture its explicit raw browser backup/inventory. Do not log Skill body.
- Stop writes, create a full database backup, calculate its SHA-256, and record
  `SELECT count(*) FROM conversations WHERE metadata ? 'activeSkills'`.
- Run default dry-run first. Apply takes `SHARE ROW EXCLUSIVE`, re-counts under
  lock, removes only the `activeSkills` JSONB key, validates updated/remaining
  counts and commits. Schema head remains `090`.
- Deploy the G20.9 image, reload twice, and prove localStorage/IndexedDB marker-
  last purge and server restart do not resurrect selection. Check historical
  content plus the single retirement label.
- Package Skills are the sole eligible Skill path after source cutover, but
  production remains disabled while the exact host is
  `ISOLATION_UNAVAILABLE`. No browser/API/rootful fallback.
- Rollback is all-path: disable writes, restore the exact full database/browser
  backup and previous images inside the declared window. Never run a synthetic
  down SQL or restore only legacy definitions into a G20.9 process.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| script invoked without variables | dry-run count plus explicit rollback; no mutation |
| backup fingerprint missing/malformed | `LEGACY_SKILL_BACKUP_FINGERPRINT_REQUIRED`; transaction aborts |
| expected count stale | `LEGACY_SKILL_EXPECTED_COUNT_MISMATCH`; transaction aborts |
| apply succeeds but key remains | `LEGACY_SKILL_SELECTION_REMAINS`; transaction aborts |
| browser write fails before marker | compensate prior writes; marker absent; reload retries |
| host isolation is not accepted | keep Runtime/Shadow execution off and return `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good**: manifest/browser/DB backups match, locked count applies exactly,
  unrelated rows are equivalent, clean restart shows zero resurrection, and
  restore rehearsal succeeds before promotion review.
- **Base**: count is zero; dry-run and apply with expected `0` are idempotent,
  no migration is added, and production remains held.
- **Bad**: guess the count, accept an arbitrary backup label, run UPDATE without
  lock, modify `updated_at`/message content, partially restore, or enable a
  fallback executor because rootless isolation is unavailable.

### 6. Tests Required

- Offline gate: focused Go/Vitest/typecheck, deleted paths/static assets,
  allowlisted historical guards only, no resolver/prompt context, SQL
  signatures and held Runtime.
- PostgreSQL 17 gate: schema `001 -> 090`, sanitized fixtures, real full dump
  fingerprint, dry-run, both rejected guards, exact JSONB deletion, unrelated
  byte-equivalence, repeated expected-zero apply, and head `090`.
- Production evidence additionally requires browser reload, backend restart,
  paired restore/rollback, clean-copy and exact-host canary/Isolation Acceptance;
  disposable gate success alone is not promotion.

### 7. Wrong vs Correct

#### Wrong

```text
deploy new image -> best-effort UPDATE -> discover count afterward -> enable API fallback
```

#### Correct

```text
G20.8 browser backup + full DB backup/fingerprint/count -> dry-run -> locked apply
-> G20.9 reload/restart proof -> full restore rehearsal -> exact-host promotion later
```
