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
bash mm-chat/scripts/verify-agent-runtime-g21-0.sh
bash mm-chat/scripts/verify-agent-root-canary-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-1.sh
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
  drills must peel the empty product tail and return to current head `094`.
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
- Every prior PostgreSQL tail drill must peel `091`, `090`, `089` and then `088`
  before testing its older guard and finish reapplied at head `094`.
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
  `090` before its original guard and returns to head `094`.
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
- All older Agent/MCP/Assistant/Skill PostgreSQL drills peel empty `091`, then
  empty `090`, before their original guards and finish at head `094`.
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
  counts and commits. Schema head is now `092`.
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
  byte-equivalence, repeated expected-zero apply, and head `094`.
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

## Scenario: Evaluate G20.10 production closure

### 1. Scope / Trigger

Apply when changing Agent Runtime capacity/budget defaults, retention,
observability, exact-host promotion evidence, incident response, rotation,
backup/restore/DR or final temporary-evidence cleanup. This contract evaluates
read-only evidence; it does not enable Runtime.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-production-closure.sh
bash mm-chat/scripts/verify-agent-production-closure.sh \
  --record /secure/operator-evidence/agent-production-closure.json
```

Policy:
`mm-chat/config/agent-runner/production-policy.json`.
Schemas:
`neo-agent-production-policy.schema.json` and
`neo-agent-production-closure.schema.json`.
Evaluator: `mm-chat/scripts/evaluate-agent-production-closure.py`.

### 3. Contracts

- The policy freezes conservative single-server capacity, root/Child/Cron
  budgets, no-Egress/no-Secret canary, retention, bounded cleanup, exact metric
  labels and alert thresholds. It never widens a frozen Grant.
- A closure record binds one Git commit, migration head `094`, Runner manifest/
  binary, Runtime Bundle, target deployment and exact policy SHA-256 to 16
  unique live checks. The input cannot self-assert its final verdict.
- `template` evidence never promotes. Production requires every check passed,
  current review, non-placeholder fingerprints, approved review and zero
  temporary canary Run/Draft/Artifact/raw-Run/Sandbox/Scratch residue.
- Records contain fingerprints, UTC times and stable result/detail codes only.
  Raw logs, content, paths, URLs, object keys, credentials, tokens and Secret
  values are forbidden.
- Exit `0` is `PROMOTION_READY`, exit `3` is an honest held decision, and exit
  `2` is invalid evidence. A ready decision is necessary but never activates a
  worker by itself.
- Retain unresolved `outcome_unknown`, object drift, failed reap and failed
  cleanup until explicit content-free incident resolution. Generic terminal
  Run pruning must exclude unresolved incident authority.
- Actual records live outside Git and runtime object namespaces. The evaluator
  accepts only bounded, non-symlink, non-group/world-writable regular files and
  performs no mutation.
- G20.10 itself adds no migration, Compose/startup worker, production adapter,
  flag, browser/API/rootful fallback or live-host change. The current closure
  contract binds head `094`; the current host stays `ISOLATION_UNAVAILABLE`.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| exact-host check is unavailable | exit `3`, `PROMOTION_HELD`, `ISOLATION_UNAVAILABLE` |
| evidence class is `template` | exit `3`, `NON_PRODUCTION_EVIDENCE` |
| review window is expired/not current | exit `3`, `EVIDENCE_STALE` |
| any live check failed/not run | exit `3`, stable held reason |
| temporary evidence/residue count is nonzero | exit `3`, `TEMPORARY_EVIDENCE_REMAINS` |
| policy SHA, migration head or release binding drifts | exit `2`, invalid evidence |
| check set is missing or duplicated | exit `2`, invalid evidence |
| record/policy is symlink or group/world writable | exit `2`, unsafe document |
| all exact production conditions pass | exit `0`, `PROMOTION_READY`; activation remains separate |

### 5. Good / Base / Bad Cases

- **Good**: exact target produces a fresh content-free `production` record,
  every live check passes, temporary evidence cleanup is zero and the evaluator
  returns `PROMOTION_READY` before a separate activation review.
- **Base**: offline self-tests and all disposable PostgreSQL gates pass, but the
  committed template returns `ISOLATION_UNAVAILABLE`; production stays off.
- **Bad**: edit a template to `passed`, override evaluation time, embed raw
  logs/secrets, reuse an old policy hash, delete incident rows or enable a
  fallback because the host is unavailable.

### 6. Tests Required

- Validate both Draft 2020-12 schemas, positive/negative fixtures, unknown root
  rejection and exact policy-fixture SHA binding through Phase 0.
- Self-test ephemeral positive semantics plus isolation-held, template-held,
  stale, cleanup residue, policy drift, incomplete, duplicate, malformed,
  writable-file and symlink cases.
- Run every Agent source and PostgreSQL 17 gate through schema head `094`, then
  full frontend/backend/RAG standalone verification.
- Run `verify-agent-runner-host.sh` separately and require nonzero
  `ISOLATION_UNAVAILABLE` here. Only a real target-host pass may back a
  production record.
- Confirm `data/`, `secrets/`, `backup/` and `.env.single-server` were not read
  or changed by the evaluator.

### 7. Wrong vs Correct

#### Wrong

```text
offline fixture says passed -> enable Runtime -> call it production evidence
```

#### Correct

```text
offline contract green + exact-host/live matrix + release/policy-bound record
-> read-only PROMOTION_READY -> separate activation decision
```

## Scenario: Deploy the G21.0 exact-host bundle and control profile

### 1. Scope / Trigger

Apply when building/installing Runner host artifacts, configuring the private
Runner endpoint/mTLS, enabling the `agent-runtime-control` Compose profile or
running production preflight. The development host is not an approved target.

### 2. Signatures

```bash
bash mm-chat/scripts/build-agent-runner-bundle.sh \
  --output /secure/release/neo-runner-g21.0 \
  --release-commit "$RELEASE_COMMIT" \
  --evidence-class production \
  --release-manifest /secure/release/release-manifest.json
python3 mm-chat/scripts/verify-agent-runner-bundle.py \
  --bundle /secure/release/neo-runner-g21.0 --expected-class production
bash mm-chat/scripts/preflight-single-server.sh /secure/mm-chat.env
bash mm-chat/scripts/verify-agent-runtime-g21-0.sh
```

Compose profile: `agent-runtime-control`. Host service:
`deploy/agent-runner/neo-runnerd.service`.

### 3. Contracts

- The deterministic bundle contains exactly seven payloads: static
  `neo-runnerd`, static `neo-runner-probe`, approved release manifest, seccomp,
  systemd unit, env template and operator README. Its canonical inventory binds
  path, mode, size and SHA-256 plus commit/head/toolchain/OS/architecture.
- The builder writes only a new explicit derived directory, uses cached Go 1.25
  with module network resolution off and never installs, provisions or starts
  host state. Production requires clean exact Git `HEAD` and non-placeholder
  approved manifest.
- Runner bind is a literal loopback/RFC1918/ULA address, never wildcard,
  hostname, link-local or public, and uses an explicit numeric port in
  `1..65535`. Systemd probes the exact manifest before start and owns mode-0700
  state/runtime directories.
- Bundle verification validates the complete embedded Runner release contract,
  not only its payload hash and approval bit; an internally consistent bundle
  with an invalid runtime/network/identity tuple still fails.
- Production Compose preflight requires a sixth distinct PostgreSQL principal,
  private literal HTTPS Runner RPC endpoint, bounded timing/batch values,
  owner-secure non-symlink evidence/mTLS files, a matching client cert/key and
  `ACTIVATION_READY`. Every execution-stage flag stays false.
- The profile has no port, read-only root, `cap_drop: ALL`, no Provider/object/
  Redis/MCP credential, only the private application network and non-creating
  read-only bind mounts.
- Disabled defaults do not read evidence/mTLS files or dial Runner. Stop the
  profile or set its control flag false for application rollback; do not delete
  Runner state/evidence.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| output exists/symlink or payload extra/missing/drifted | bundle build/verify rejects |
| production source dirty/commit mismatch/manifest placeholder | production bundle rejects |
| embedded release contract invalid after inventory rehash | bundle rejects |
| wildcard/hostname/public/link-local Runner bind | daemon/preflight rejects before listen |
| shared DB user/password or execution flag true | preflight rejects without printing secret |
| cert/key mismatch, insecure/symlink file or non-READY record | preflight rejects |
| profile not selected/control false | no worker, file read or Runner dial |
| current host probe | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good:** operator verifies one clean production bundle, installs it on the
  approved non-root host, captures fresh evidence, passes preflight and starts
  only the control profile.
- **Base:** checked-in env keeps every Agent flag false and the unapproved
  manifest builds/verifies only as `template`.
- **Bad:** turn the development machine into Runner, use rootful Docker/public
  proxy, let Compose create missing secret paths, or enable Root Run because
  control maintenance is healthy.

### 6. Tests Required

- Run bundle determinism plus symlink/extra/mode/size/hash/unapproved production
  negatives.
- Render Compose with example and production env; assert default-off, exact
  profile/network/mount/capability/credential boundary and no host port.
- Run production preflight positive/negative mTLS/evidence/principal/endpoint/
  flag cases without touching `.env.single-server` or protected runtime paths.
- Run `verify-agent-runtime-phase0.sh`, `verify-agent-runtime-g21-0.sh` and
  `verify-standalone.sh --full`; host acceptance remains separately nonzero.

### 7. Wrong vs Correct

#### Wrong

```text
template bundle -> Compose Root Run worker -> public Runner/rootful fallback
```

#### Correct

```text
clean content-addressed production bundle -> exact non-root target acceptance
-> fresh control_plane activation -> control-only Compose profile
-> execution stages remain physically disabled
```

## Scenario: Deploy the G21.1 Root canary profile

### 1. Scope / Trigger

Apply when configuring the `agent-runtime-root-canary` Compose profile,
separate mTLS/authority material, seventh PostgreSQL principal or G21.1
preflight. The development host remains ineligible.

### 2. Signatures

```bash
bash mm-chat/scripts/preflight-single-server.sh /secure/mm-chat.env
bash mm-chat/scripts/verify-agent-root-canary-activation.sh
bash mm-chat/scripts/verify-agent-root-canary-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-1.sh
```

Compose profile: `agent-runtime-root-canary`. Runner identity:
`spiffe://neo-chat/agent-runtime-root-canary`.

### 3. Contracts

- G21.0 control must already be enabled and ready. Canary reuses neither its
  certificate/key nor its database LOGIN.
- Mount exactly nine non-creating read-only files: canary certificate/key,
  server CA, release manifest, production policy, canary activation, immutable
  plan and Ed25519 private/public authority keys.
- The canary LOGIN recursively inherits exactly
  `agent_orchestrator_runtime,agent_runner_control`, has no elevated attribute
  or direct table DML and is distinct from all six earlier principals.
- Preflight requires a private literal HTTPS Runner endpoint, exact canary
  identity, secure regular files, matching certificate/key and Ed25519 keys,
  no-Egress/no-Secret/empty-Tool plan and a ready `root_run_canary` record.
- A production `root_run_canary` evaluator invocation must provide both
  `--canary-plan` and `--authority-public-key`; their optional parser defaults
  exist only so checked-in template/held evidence can be evaluated without
  local production material. Missing either binding fails as `WIRING_INVALID`.
- The profile has no port, read-only root, `cap_drop: ALL`, private network
  only and no Provider/object-store/Redis/MCP/vault secret. Broad Runtime,
  Broker, Child, Scheduler, Skill-install and Learning flags stay false.
- Roll back by stopping only the canary profile or setting its flag false.
  Keep G21.0 control available for reconcile; never delete runtime state,
  reconstruct lease tokens or hand-edit terminal projections.
- Source/disposable gates do not activate production. Only fresh exact-target
  evidence may enable the profile; the current host remains
  `ISOLATION_UNAVAILABLE`.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| profile not selected/canary false | no mounted-file read or Runner dial |
| canary true while control false | preflight rejects |
| shared identity/principal or extra inherited role | preflight/startup rejects |
| insecure/symlink/mismatched mTLS or authority key | preflight rejects |
| plan contains Tool/Egress/Secret or network | preflight rejects |
| activation template/stale/drifted/widened/residue | not READY; no worker start |
| current development host | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good:** exact target keeps control active, mounts separate reviewed canary
  material, passes preflight and runs only one synthetic Root canary.
- **Base:** both profiles are unselected and flags false; no protected file is
  read and no host is modified.
- **Bad:** enable broad Runtime, mount a credential directory, reuse control
  cert/login, expose a port or install rootful/container-socket fallback.

### 6. Tests Required

- Render example and production Compose with the canary profile; assert nine
  mounts, default-off flags, no ports/credentials, private-only network,
  hardened process settings and no production build block.
- Run preflight default and canary-without-control negative, plus exact enabled
  activation/plan/mTLS/key/principal cases in the focused G21.1 verifier.
- Run disposable PostgreSQL 17 atomic/role proof, G21.0 regression, Phase 0 and
  full standalone. Require exact-host acceptance to remain separately nonzero
  on this machine.
- Build the disposable migration helper with `-buildvcs=false`: standalone
  verification runs from a source-only tar copy without `.git`, and that gate
  must validate schema behavior rather than require unavailable VCS stamping.

### 7. Wrong vs Correct

#### Wrong

```text
G21.0 control profile -> reuse certificate/login -> enable broad Runtime
```

#### Correct

```text
ready G21.0 control + separate Root-canary files/login/profile
-> exact preflight -> one synthetic Run -> disable canary, retain reconcile
```

## Scenario: Deploy the G21.2 Broker and Artifact canary profile

### 1. Scope / Trigger

Apply when configuring `agent-runtime-broker-canary`, its private relay network,
eighth PostgreSQL principal, object-store/MCP material, strict plan or G21.2
preflight. The development host remains ineligible.

### 2. Signatures

```bash
bash mm-chat/scripts/preflight-single-server.sh /secure/mm-chat.env
bash mm-chat/scripts/verify-agent-broker-canary-activation.sh
bash mm-chat/scripts/verify-agent-broker-canary-preflight.sh
bash mm-chat/scripts/verify-agent-artifact-publication-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-2.sh
```

Compose profile: `agent-runtime-broker-canary`. Caller identity:
`spiffe://neo-chat/agent-runtime-broker-canary`. Relay identity:
`spiffe://neo-chat/neo-runner-broker-relay`.

### 3. Contracts

- G21.0 control and G21.1 Root canary must already be enabled and ready. The
  Broker canary reuses neither identity, TLS private key nor database LOGIN.
- The LOGIN recursively inherits exactly `agent_orchestrator_runtime`,
  `agent_runner_control`, `agent_effect_control`, `agent_artifact_control`, has
  no elevated attribute/direct table DML and is distinct from earlier principals.
- Mount exactly the reviewed canary TLS/evidence/plan/authority/relay files,
  read-only Project/Workspace roots and one writable quarantine root. Only the
  MCP token is a Docker Secret. All bind mounts disable host-path creation.
- Broker service may receive its dedicated database URL, object-store
  credential and MCP token file. Runner receives only relay URL/client TLS/CA;
  Sandbox receives neither side's credentials and stays `networkMode=none`.
- Relay uses the exact internal `172.31.254.0/29` network, service address
  `172.31.254.2` and literal HTTPS path
  `/internal/agent-broker/v1/relay`. No hostname, wildcard, public address or
  host port is accepted.
- Preflight requires distinct exact identities, private Runner/relay endpoints,
  RPC timeout `1s..10s`, authority TTL `10s..15s`, existing S3-compatible
  bucket with `S3_BUCKET_AUTO_CREATE=false`, private MCP Runner URL/token,
  matching TLS/key pairs, immutable roots, strict five-action plan and a ready
  `broker_artifact_canary` record at migration head `094`.
- Broad Runtime, generic Broker mutation/read, Child, Scheduler, Skill-install
  and Learning flags stay false. Roll back by stopping only this profile or
  clearing its flag; retain migration `091`, rows/objects and G21.0/G21.1
  reconciliation authority.
- PostgreSQL and object storage remain one backup/restore unit. Restore with the
  canary off, verify every Artifact object reference, remove only proven
  unreferenced canary objects, then require fresh exact-host activation.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| profile not selected/flag false | no G21.2 file read, DB open or Runner dial |
| G21.0/G21.1 false | preflight rejects |
| shared identity/principal or extra inherited role | preflight/startup rejects |
| relay hostname/wildcard/mismatched IP/path | preflight rejects |
| RPC/TTL out of bounds or bucket auto-create true | preflight rejects |
| insecure/symlink/mismatched TLS/key/authority material | preflight rejects |
| plan widened or activation stale/drifted/residue | not READY; no worker start |
| Runner/Sandbox receives Broker credential | source/Compose gate fails |
| current development host | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good:** exact target keeps G21.0/G21.1 ready, starts the separate G21.2
  profile with reviewed material and proves the complete Runner-relay-Broker
  canary plus zero residue.
- **Base:** all three profiles are unselected and flags false; ordinary app
  startup never reads target-host material.
- **Bad:** expose the relay, reuse Root-canary identity/login, mount a credential
  directory, enable bucket creation or treat a disposable Artifact as live proof.

### 6. Tests Required

- Render example and production Compose; assert default-off profile, static
  internal relay IP, no port, hardened process, 15 exact mounts, one writable
  quarantine, one MCP secret and production image-only wiring.
- Run default/negative and exact enabled preflight for prerequisites, identity,
  endpoint, time bounds, bucket policy, TLS/key/plan/evidence and principals.
- Run focused activation and PostgreSQL 17 gates, G21.1 regression, Phase 0 and
  full standalone. Require exact-host acceptance to remain separately nonzero.
- Prove Runner env and Sandbox plan contain no database, S3, MCP, Provider,
  vault or relay-server credential.

### 7. Wrong vs Correct

#### Wrong

```text
Root-canary identity + public relay + Runner holds DB/S3/MCP credentials
```

#### Correct

```text
ready G21.0/G21.1 + distinct G21.2 caller/LOGIN + private relay identity
-> Broker-only credentials -> exact five-action canary -> disable and reconcile
```

## Scenario: Deploy the G21.3 Project mutation canary profile

### 1. Scope / Trigger

Apply when configuring `agent-runtime-project-canary`, the Project relay
network, ninth database principal, 14 secure mounts, signed approval or G21.3
preflight/rollback. The current development host remains ineligible.

### 2. Signatures

```bash
bash mm-chat/scripts/preflight-single-server.sh /secure/mm-chat.env
bash mm-chat/scripts/verify-agent-project-canary-activation.sh
bash mm-chat/scripts/verify-agent-project-canary-preflight.sh
bash mm-chat/scripts/verify-agent-project-mutation-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-3.sh
```

- Compose profile: `agent-runtime-project-canary`.
- Caller: `spiffe://neo-chat/agent-runtime-project-canary`.
- Relay: `spiffe://neo-chat/neo-runner-project-relay`, network
  `172.31.254.8/29`, service `172.31.254.10:9445`.
- Principal: `agent_project_canary_app` with exactly four inherited control
  roles and no owner/DML/provision authority.

### 3. Contracts

- G21.0 control, G21.1 Root and G21.2 Broker/Artifact evidence/flags must be
  ready while all broad Runtime/Broker mutation/MCP write/Egress/Secret/Child/
  Cron/Skill-install/Learning flags remain false.
- The Project caller, Project relay identity, Project endpoint and ninth LOGIN
  are distinct from every prior caller/relay/principal. Broker and Project
  relay CIDRs cannot overlap. Use one literal private HTTPS endpoint with no
  host port, hostname, wildcard, public or link-local address.
- Mount exactly 14 read-only, owner-secure, regular non-symlink files:
  Runner client cert/key/CA, release manifest, policy, activation, plan,
  authority private/public keys, approval document/public key, relay server
  cert/key/client CA. Disable host-path creation. Use no Compose Secret.
- Authority, approval and TLS sensitive materials are pairwise distinct.
  Approval private key stays offline and outside Git. The canary receives no
  S3, MCP, Provider, vault, Redis or generic Egress configuration.
- Runner env receives only the Project caller plus outbound relay URL/client
  cert/key/server CA/server name/relay identity. It must not contain Project DB,
  approval document/key, authority private key or relay server key. Sandbox
  plan remains `networkMode=none`, capability-free and credential-free.
- Preflight validates a strict one-action current-window plan, Ed25519-signed
  approval, current head `094`, stable activation binding fingerprint, exact
  release/target/caller/request/action/actor/window, TLS/key pairs and ninth
  principal before any database or Runner access.
- Operator provisions one reviewed synthetic baseline while all canaries are
  off. Runtime cannot provision arbitrary resources. Stop/disable only this
  profile for rollback; retain migration `092` and immutable facts.
- Restore with every canary off. Matching committed facts may drive baseline
  cleanup only; never rerun CAS. Fresh approval and activation are required
  before restart. Destructive down is allowed only after archived evidence,
  removal of the ninth LOGIN membership and joint operator truncate of all
  three G21.3 tables.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| profile unselected/flag false | no G21.3 secure file read, DB open or Runner dial |
| any G21.0-G21.2 prerequisite false | preflight rejects |
| broad mutation/MCP/Egress/Secret flag true | preflight/startup rejects |
| shared caller/relay/principal/key or overlapping CIDR | preflight rejects |
| hostname/public/wildcard/link-local/incorrect relay path | preflight rejects |
| insecure/symlink/missing/placeholder/mismatched file | reject before DB/Runner access |
| approval signature/binding/window drift | activation/preflight rejects |
| Runner/Sandbox contains controller credential | source/Compose gate fails |
| populated migration down | `AGENT_PROJECT_MUTATION_DOWN_REQUIRES_EMPTY` |
| current development host | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good:** approved target preserves earlier profiles, starts only G21.3 with
  exact secure inputs, runs one CAS, restores baseline and retains zero runtime
  residue plus content-free immutable facts.
- **Base:** Project profile is absent from default Compose and all flags are
  false; ordinary services never read G21.3 material.
- **Bad:** reuse Broker relay/TLS/login, mount a credential directory or
  approval private key, auto-create Projects, retry after ambiguity or treat a
  disposable PostgreSQL pass as live activation.

### 6. Tests Required

- Render development/production/default Compose; assert default-off profile,
  hardened process, exact static network/IP, 14 read-only mounts, no secrets,
  production image-only wiring and broad flags false.
- Run enabled/default/negative preflight for prerequisites, exact identities,
  principals, CIDRs, endpoints, secure files, TLS/Ed25519, plan, approval and
  activation.
- Prove Runner env/Sandbox plan credential absence and exact outbound relay
  tuple only.
- Run focused Project PostgreSQL 17 drill, G21.2 regression, Phase 0,
  standalone full and separately expected-nonzero exact-host gate.
- Backup/restore must preserve synthetic resource/receipt/cleanup counts and
  require cleanup/status reconciliation before a fresh activation.

### 7. Wrong vs Correct

#### Wrong

```text
Broker relay/login reused + approval private key mounted + timeout => new CAS key
```

#### Correct

```text
ready G21.0-G21.2 + isolated Project caller/relay/LOGIN + offline signed approval
-> one synthetic CAS/status/cleanup -> disable profile, preserve immutable facts
```

## Scenario: Deploy the G21.4 depth-one Child canary profile

### 1. Scope / Trigger

Apply when configuring `agent-runtime-child-canary`, its fifth Runner caller,
tenth database principal, nine secure mounts, strict Parent/Child plan or G21.4
activation/preflight/rollback. The development host remains ineligible.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-child-canary-activation.sh
bash mm-chat/scripts/verify-agent-child-canary-preflight.sh
bash mm-chat/scripts/verify-agent-child-canary-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-4.sh
```

- Compose profile: `agent-runtime-child-canary`.
- Caller: `spiffe://neo-chat/agent-runtime-child-canary`.
- Principal: `agent_child_canary_app`, inheriting exactly three control roles.

### 3. Contracts

- G21.0 control, G21.1 Root, G21.2 Broker/Artifact and G21.3 Project evidence/
  flags must be ready. Broad Runtime/Broker/MCP/Egress/Secret/Scheduler/
  Skill-install/Learning flags remain false. Child and delegation flags match.
- The fifth caller and tenth LOGIN are distinct from every earlier identity and
  principal. Its Runner methods are lifecycle-only; no Prepare/Commit or relay.
- Mount exactly nine owner-secure, regular non-symlink files read-only: client
  cert/key/CA, release manifest, policy, activation, plan and authority
  private/public keys. Sensitive material does not reuse an earlier stage.
- Use no ports, Compose Secret, relay, S3, MCP, Provider, vault or Redis
  credential. Runner receives only the Child caller identity. Sandboxes remain
  credential-free and `networkMode=none`.
- Preflight validates migration head `094`, private exact Runner endpoint,
  mTLS/key pairs, distinct principals/material, stable plan identities, exact
  one-Parent/one-Child shape, empty derived Child Registry, strict expiry and
  four-budget subset, fixed argv and no network/capability.
- Reconcile failed reaps and existing lineage before work. Wait lost token and
  launch authority expiry, remove only the exact Child from Runner, atomically
  close the reap, then cancel Parent. Health requires zero reap/Runner residue.
- Rollback stops only this profile and clears Child/delegation flags. Retain
  migration `093` and immutable facts. Down is disposable-only after its clean
  guard passes.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| profile absent or either Child/delegation flag false | no G21.4 file, DB or Runner access |
| any G21.0-G21.3 prerequisite false | preflight rejects |
| caller/principal/key reuse or extra role | preflight/startup rejects |
| plan second Child/depth two/widening/network/argument | plan/preflight rejects |
| Runner/Sandbox receives controller or Broker credential | source/Compose gate fails |
| activation held/stale/drifted/residue | not READY; no worker start |
| current development host | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Good / Base / Bad Cases

- **Good:** the isolated profile preserves all earlier callers, launches one
  exact lineage, completes Child-first cleanup and leaves no Runner residue.
- **Base:** the profile is absent from default Compose and both Child and
  delegation flags are false; ordinary services never read G21.4 material.
- **Bad:** reuse an earlier caller/LOGIN, mount Broker credentials, use a
  public Runner endpoint, or classify a disposable PostgreSQL pass as
  exact-host isolation evidence.

### 6. Tests Required

- Render default, development and production Compose; assert default-off,
  hardened private service, nine read-only mounts, no secret/port and
  production image-only wiring.
- Run activation and enabled preflight identity/principal/TLS/plan/credential
  negatives plus exact READY synthetic evidence.
- Run focused restart/race tests and PostgreSQL 17 exact inventory, late-launch,
  atomic completion, least privilege, dump/restore and guarded down/up.
- Run G21.0-G21.3 regression, Phase 0, standalone full and separately
  expected-nonzero exact-host acceptance.
- Disposable Docker drills that publish PostgreSQL on a dynamically assigned
  host port must resolve `docker port` again after `docker restart` before any
  host-side migrator connects; container-internal `psql` readiness does not
  prove the old host URL is still valid.

### 7. Wrong vs Correct

#### Wrong

```text
reuse Root caller/login -> give Child Prepare/Commit -> restart creates sibling
```

#### Correct

```text
ready G21.0-G21.3 + distinct lifecycle-only caller/LOGIN + exact single lineage
-> Child-first authority wait/reap -> atomic durable completion -> Parent cancel
```

## Scenario: Deploy the G21.5 exact Cron and Draft-learning profiles

### 1. Scope / Trigger

Apply when configuring either G21.5 profile, its eleventh/twelfth database
principal, activation/plan evidence, Draft Runner caller/object credential,
preflight, backup/restore or rollback. The development host remains ineligible.

### 2. Signatures

```bash
bash mm-chat/scripts/verify-agent-runtime-g21-5-preflight.sh
bash mm-chat/scripts/verify-agent-cron-worker-postgres17.sh
bash mm-chat/scripts/verify-agent-draft-learning-worker-postgres17.sh
bash mm-chat/scripts/verify-agent-runtime-g21-5.sh
```

- Profiles: `agent-runtime-cron-worker` and
  `agent-runtime-draft-learning-worker`.
- Flags: `AGENT_CRON_WORKER_ENABLED` and
  `AGENT_DRAFT_LEARNING_WORKER_ENABLED`.
- Draft caller: `spiffe://neo-chat/agent-runtime-draft-learning`.
- Principals: `agent_cron_worker_app` and
  `agent_draft_learning_worker_app`, each inheriting one worker role.

### 3. Contracts

- Require fresh READY G21.0-G21.4 evidence, exact migration head `094`, exact
  plan SHA and non-placeholder production fingerprints. Profiles and flags are
  independent and default off.
- Keep broad Runtime, Runner control, Scheduler, Learning, Skill install,
  Broker read/mutation, API/Chat and product cohort flags false. Starting one
  profile must not read the other stage's files or open its database/Runner/
  object path.
- Cron receives only its exact database URL and reviewed activation/plan/
  prerequisite files. It has no Runner TLS/key, authority key, S3, Provider,
  administrator or product credential.
- Draft receives a distinct database URL, Runner client mTLS, authority key
  pair, strict plan and dedicated S3-compatible cleanup credential. Runner host
  env and Sandboxes receive none of the database/S3/private authority material.
- Require a literal private HTTPS Runner URL, exact server name and caller,
  private regular non-symlink files, matching TLS/key pairs and distinct
  sensitive material. Fixed Sandboxes remain rootless, read-only,
  capability-free and `networkMode=none`.
- Runner grants the Draft caller Probe/List/Reconcile/Launch/Result/Cancel only
  and preserves all five earlier caller policies plus both relay routes.
- Provision exactly one reviewed active/due Template and one reviewed
  quarantined Draft with pre-staged Workspace. Cron is Runner-free; Draft uses
  separate isolation/evaluation Sandboxes and cannot Promote.
- Rollback stops/disables one profile and reconciles only its exact target.
  Retain migration `094`, activation, human decision, receipt and cleanup facts.
- Restore PostgreSQL and object storage as one set with both profiles off.
  Reconcile exact Cron claims, wait Draft lease/authority expiry, remove exact
  Sandbox residue, preserve object-before-row cleanup and require fresh
  activation before restart.

### 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| profile absent or flag false | no stage file read, DB open or Runner/object call |
| any prerequisite false, head/plan/evidence drift | preflight rejects |
| shared principal/key/caller or extra inherited role | preflight/startup rejects |
| Cron receives Runner/S3/admin credential | source/Compose gate fails |
| Draft Runner/Sandbox receives DB/S3/private authority credential | source/Compose gate fails |
| target claim, Runner attempt or cleanup residue remains | unhealthy; no fresh activation |
| current development host | expected nonzero `ISOLATION_UNAVAILABLE` |

### 5. Tests Required

- Render default/development/production Compose and prove both profiles absent
  by default, hardened and image-only in production.
- Run default and enabled Cron-only, Draft-only and combined preflight, plus
  identity, role, credential, TLS, plan, prerequisite and held-evidence
  negatives.
- Run both focused source/race/vet and PostgreSQL 17 exact-target gates, G21.4
  regression, Phase 0, standalone full and expected-nonzero host acceptance.
- Backup/restore must preserve target/result/receipt/decision/cleanup counts and
  require zero claim/Runner/object residue before reactivation.
