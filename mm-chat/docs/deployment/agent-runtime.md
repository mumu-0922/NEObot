# Neo Agent Runtime Operations

Status: G20.1 no-execute Skill supply, G20.2 durable Orchestrator, G20.3
`neo-runnerd`, G20.4 brokered effects, G20.5 depth-1 Child delegation, G20.6
durable Cron scheduling, G20.7 Draft-only learning, G20.8 Agent Center/default-
off Shadow control, and G20.9 legacy text-Skill source retirement are
implemented. G20.10 supplies the fail-closed operations policy, evidence
contract, evaluator and incident runbooks. G21.0 adds a default-off,
control-only Runner maintenance profile and exact-host deployment bundle.
G21.1 adds a second default-off, synthetic-only Root Run canary profile.
Exact-host installation and general Root Run/Broker/Child/Scheduler/Learning/
Shadow promotion remain held. Do not enable Agent execution or install the
bundle on this development host.

## Default state

All future switches default off:

```text
AGENT_RUNTIME_ENABLED=false
AGENT_RUNNER_CONTROL_ENABLED=false
AGENT_ROOT_RUN_CANARY_ENABLED=false
AGENT_SCHEDULER_ENABLED=false
AGENT_SKILL_INSTALL_ENABLED=false
AGENT_LEARNING_ENABLED=false
AGENT_DELEGATION_ENABLED=false
AGENT_BROKER_READ_ONLY_ENABLED=false
AGENT_BROKER_MUTATION_ENABLED=false
```

The host-only `deploy/agent-runner/neo-runnerd.env.example` is not an activation
file. The `agent-runtime-control` and `agent-runtime-root-canary` Compose
profiles exist only for an approved target and are not selected by default.
With their flags false, ordinary startup neither reads target-host evidence,
plan, mTLS or authority-key files nor dials Runner.

Runtime disabled must block discovery installation changes, new Runs, leases,
Cron triggers, Child Runs and Draft promotion as appropriate, but must not stop
retention, expired-intent cleanup, Artifact deletion, orphan reconcile or audit
access.

## Target topology

```text
single-server application network
  Go API / PostgreSQL / Redis / MinIO
          |
          | dedicated mTLS control endpoint
          v
host non-root neo-runnerd service account
          |
          | rootless OCI user namespace + delegated cgroup v2
          v
one fresh Sandbox per Run
  + immutable package/runtime rootfs
  + Project snapshot (not host bind)
  + bounded tmpfs Scratch
  + private broker channels only
```

`neo-runnerd` is not the existing `mcp-runner`. The latter starts admitted MCP
stdio Tool servers inside a hardened container; Agent Runner owns arbitrary
per-Run Skill workload lifecycle and therefore needs stronger rootless OCI,
lease and per-Run isolation evidence. Neither service may receive Docker socket,
database, object-store or Provider vault credentials.

## Host prerequisites and capability probe

The target Linux host must provide:

- kernel user namespaces and nonzero `user.max_user_namespaces`;
- dedicated subuid/subgid range for the Runner service account;
- cgroup v2 with delegation that works for an unprivileged service;
- an approved rootless OCI stack and pinned executable versions;
- seccomp enforcement and an exact reviewed profile;
- a rootless network path that can enforce `none` or Broker-only egress;
- a snapshot/copy/idmapped Workspace mechanism with no arbitrary host bind;
- exact process/cgroup kill and orphan reconciliation;
- read-only immutable runtime images and content-addressed package storage.

The deployment probe must run as the exact Runner service account. It records
kernel, runtime/storage/network versions, namespace mapping, cgroup controllers,
seccomp status, probe-suite fingerprint and result without printing host paths
or secrets. Any missing feature or drift makes Runner readiness false. Falling
back to rootful Docker, `sudo`, `--privileged`, unconfined seccomp, added
capabilities or host networking is forbidden.

Current development-host observation (2026-08-13): the checked-in exact release
probe exits nonzero with `ISOLATION_UNAVAILABLE`. Its sanitized classes include
unapproved release, exact Podman/crun/conmon/uidmap drift, missing writable
delegated cgroup v2 and seccomp-profile drift. Rootful Docker availability is
irrelevant and is never promoted. This is the required fail-closed G20.3 result;
the implementation does not install or change the host.

## Service account and filesystem boundary

Future deployment must use a dedicated non-login `neo-runner` identity, not the
Go API or database user. Required writable state is minimal and separated:

```text
runtime cache: immutable-digest keyed; no source checkout
run staging: one Runner-owned directory per Attempt; owner/mode checked
scratch: noexec/nosuid/nodev tmpfs with exact byte bound; host broker staging
         is removed on all terminal/orphan/failure paths
probe evidence: bounded metadata only
mTLS key: mode-0600 secret file, not env/Git/image
```

Package candidates are fetched/admitted by the Control Plane into quarantine.
Runner consumes only admitted immutable object coordinates and verifies their
fingerprints again. Runner must never `git clone`, resolve floating dependencies
or run package installers from a live source at Run time.

## mTLS control plane

- bind only explicit loopback/private TCP; G20.3 template uses loopback;
- separate CA/service identities for API and Runner;
- mode-0600 private keys from mounted secret files;
- rotate by overlapping trust roots, restarting one side at a time, then
  removing the old root after reconciliation;
- TLS 1.3, version negotiation and `probe` must pass before readiness;
- nonce/request replay state is durable or otherwise cannot be reset into an
  acceptance window by ordinary process restart;
- logs include request/Run/Attempt IDs and error class only.

No public reverse-proxy route exposes Runner RPC. Frontend never receives its
URL, certificate, nonce, lease token, package coordinate or Secret handle.

The Backend-facing `agentrunner.RPCClient` pins the Runner CA and exact server
identity, sends strict `neo.runner-rpc/v1`, bounds headers/body/deadline and
accepts only a request-ID/nonce/method-bound response. There is no bearer token
fallback.

## G20.3-G21.1 source and verification commands

```bash
bash scripts/verify-agent-runner.sh
bash scripts/verify-agent-runner-postgres17.sh
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
bash scripts/verify-agent-runtime-phase0.sh
bash scripts/verify-agent-runner-host.sh
```

The source and disposable PostgreSQL commands prove control-plane contracts
only. The host command must exit nonzero on this host and
print `ISOLATION_UNAVAILABLE` with
content-free failure classes. Only an approved manifest installed for the exact
`neo-runner` account and a complete passing target-host suite may emit a ready
marker. The templates under `deploy/agent-runner/` are review artifacts only;
they do not create the account, directories, sub-IDs, cgroup delegation or
certificate files.

Even an otherwise ready Podman host remains unavailable until the manifest-bound
Isolation Acceptance report is approved, generated by the exact current UID/GID
and release tuple, contains every required suite feature and is no older than
24 hours. Replacing a report without advancing its manifest SHA-256 is drift.

Do not apply service-level `NoNewPrivileges=yes` or an empty capability bounding
set to this rootless Podman service: either blocks pinned `newuidmap`/
`newgidmap`. The unit exposes only `CAP_SETUID`/`CAP_SETGID` in its bounding set,
keeps Ambient capabilities empty, and proves the Sandbox itself has empty
capabilities plus `no-new-privileges` before start.

## G21.0 exact-host bundle and control activation

Build one derived bundle without installing it:

```bash
bash scripts/build-agent-runner-bundle.sh \
  --output /secure/release/neo-runner-g21.0 \
  --release-commit "$RELEASE_COMMIT"
python3 scripts/verify-agent-runner-bundle.py \
  --bundle /secure/release/neo-runner-g21.0 \
  --expected-class template
```

Production mode additionally requires `--evidence-class production`, an exact
clean Git `HEAD`, and a separately reviewed manifest with `approved=true` and
no zero fingerprint. The builder uses cached Go 1.25 modules/toolchain with
network module resolution disabled, writes only to a new explicit output path,
and never creates accounts, subordinate IDs, cgroups, certificates or systemd
state. Operators copy files only after verifying the content-addressed
inventory. `neo-runnerd` accepts literal loopback/RFC1918/ULA addresses only;
the systemd unit runs the exact manifest probe before start and owns mode-0700
state/runtime directories.

Generate the external `control_plane` activation record after the exact target
passes isolation, clean-copy install, private mTLS probe, zero-inventory
reconcile and rollback/Kill-Switch checks. Validate without mutation:

```bash
python3 scripts/evaluate-agent-production-activation.py \
  --record /secure/operator-evidence/agent-production-activation.json \
  --policy config/agent-runner/production-policy.json \
  --release-manifest /secure/release/release-manifest.json \
  --runner-binary /secure/release/neo-runner-g21.0/bin/neo-runnerd \
  --client-certificate /secure/agent-control/client.crt \
  --server-ca /secure/agent-control/server-ca.crt \
  --endpoint 'https://10.0.0.8:9443/internal/neo-runner/v1/rpc' \
  --runner-id neo-runner-primary \
  --server-name neo-runner.internal \
  --caller-identity spiffe://neo-chat/agent-runtime-control \
  --release-commit "$RELEASE_COMMIT"
```

Exit `0` must report `ACTIVATION_READY`. Provision a sixth distinct PostgreSQL
login that inherits only `agent_runner_control`; never reuse migrator/API/
Memory/RAG credentials. Put the client certificate/key, server CA, approved
release manifest and activation record in owner-only regular files, keep the
policy non-writable by group/world, set only
`AGENT_RUNNER_CONTROL_ENABLED=true`, run `preflight-single-server.sh`, then
select `--profile agent-runtime-control`. The service has no port, uses only the
private application network, a read-only root filesystem and `cap_drop: ALL`.
Stopping that profile or resetting its one control flag to false is the
application-side rollback; it does not delete Runner state or evidence.

Verify the whole slice with:

```bash
bash scripts/verify-agent-runtime-g21-0.sh
```

The command must end with the current-host expected failure
`ISOLATION_UNAVAILABLE`; its synthetic positive activation exists only inside a
temporary test directory and is not live evidence.

## G21.1 Root Run canary activation

G21.1 depends on a currently ready G21.0 control plane but does not reuse its
identity or database login. Provision a seventh nonprivileged LOGIN that
recursively inherits exactly `agent_orchestrator_runtime` and
`agent_runner_control`. Configure `neo-runnerd` with the additional exact
client identity `spiffe://neo-chat/agent-runtime-root-canary`; the control
identity stays limited to `probe/list/reconcile`, while the canary identity may
also call only `launch/heartbeat/cancel`. Never grant either identity Broker
`prepare/commit` by proxy or route.

Place the following as owner-only, regular, non-symlink files outside Git:

- a separate canary client certificate/key and Runner server CA;
- the exact approved Runner release manifest and production policy;
- one strict `neo.agent-root-run-canary-plan/v1` plan for a pre-provisioned
  synthetic user;
- one Ed25519 authority private/public key pair; and
- one fresh `neo.agent-production-activation/v1` record with stage
  `root_run_canary` binding every preceding file and the exact target/release.

The plan must have an empty Tool Registry, depth zero, no Egress, no Secrets,
`networkMode=none`, a read-only rootfs, empty capabilities and bounded
resources. Generate the content-free activation record only after the exact
target proves G21.0 readiness, isolation, private canary mTLS, restart
recovery, Kill-Switch rollback, signed authority, plan validity and zero
inventory. Validate it with:

```bash
python3 scripts/evaluate-agent-production-activation.py \
  --record /secure/operator-evidence/agent-root-canary-activation.json \
  --policy config/agent-runner/production-policy.json \
  --release-manifest /secure/release/release-manifest.json \
  --client-certificate /secure/agent-root-canary/client.crt \
  --server-ca /secure/agent-root-canary/server-ca.crt \
  --canary-plan /secure/agent-root-canary/root-canary-plan.json \
  --authority-public-key /secure/agent-root-canary/authority-public-key \
  --endpoint 'https://10.0.0.8:9443/internal/neo-runner/v1/rpc' \
  --runner-id neo-runner-primary \
  --server-name neo-runner.internal \
  --caller-identity spiffe://neo-chat/agent-runtime-root-canary \
  --release-commit "$RELEASE_COMMIT"
```

Require `ACTIVATION_READY` with reason
`ROOT_RUN_CANARY_GATES_PASSED`. Then set only
`AGENT_RUNNER_CONTROL_ENABLED=true` and
`AGENT_ROOT_RUN_CANARY_ENABLED=true`, run production preflight and select both
explicit profiles. Every broad Runtime, Broker, Child, Scheduler, Skill-install
and Learning flag remains false. The canary service has no port, a read-only
root filesystem, `cap_drop: ALL`, only the private network, exactly nine
non-creating read-only mounts and no Provider/object-store/Redis/MCP secret.

The worker executes the exact idempotent Run once, cooperatively cancels it and
requires zero Runner inventory. Stopping only
`agent-runtime-root-canary` or resetting its flag to false is the application
rollback; keep G21.0 control alive for reconciliation and do not delete Runner
state/evidence. A restart with a live tokenless Attempt waits for lease expiry;
operators must not reconstruct the lease token or manually mark projections
terminal. A heartbeat failure attempts exact signed cancel and atomic durable
cancellation; a cancel outage remains fenced for expiry/reconcile.

Verify source and disposable PostgreSQL behavior with:

```bash
bash scripts/verify-agent-runtime-g21-1.sh
```

This gate proves the transaction rollback/atomic terminal chain, exact role
membership, method isolation, Compose/preflight boundaries and G21.0
regression. Its expected local `ISOLATION_UNAVAILABLE` result means this host
still cannot activate the profile.

## Release order (future groups)

1. Back up PostgreSQL and object storage as a verified pair; record current
   legacy Skill inventory without copying secrets/content into logs.
2. Install/pin the reviewed rootless runtime and `neo-runnerd` artifact through
   the host package/release process.
3. Create the dedicated service account, subuid/subgid, cgroup delegation,
   filesystem and mTLS secrets; do not reuse app credentials.
4. Run the capability probe and full Isolation Acceptance Suite as that user.
5. Deploy additive PostgreSQL/object schemas with all Agent switches off.
6. Start Runner, reconcile zero/known Sandboxes and verify private readiness.
7. Start Go Backend with Runtime still globally disabled; verify cleanup and
   audit continue.
8. Admit one official read-only synthetic Skill and run an operator-only canary
   with no Egress/Secret/Project write.
9. Promote Tool/Egress/Secret, Child, Cron and learning groups separately after
   their gates.
10. Perform legacy text-Skill deletion only in the final cutover group after
    backup/restore, clean-copy, restart, history-label and rollback rehearsal.

No group combines rootless host installation, durable state migration and
legacy destructive deletion in one release.

## Brokered-effect operations boundary

Migration `086` creates the durable intent, approval, Commit receipt and
secret-handle-digest authority under `agent_effect_owner`. Only the narrow
`agent_effect_control` role may read it and call its exact mutation functions.
`go_api_runtime`, `agent_orchestrator_runtime`, `agent_runner_control` and the
host Runner gain no direct effect-table DML.

There is currently no production Broker worker, authenticated Backend-to-Runner
relay, vault resolver, Project store adapter, object publication adapter or live
mutable MCP/external executor. The Runner default relay returns
`RUNTIME_UNAVAILABLE`. Operators may run the two Broker verification commands
above against source and disposable PostgreSQL only; they must not manually
invoke migration functions to manufacture an effect or treat the deterministic
Project CAS fake as storage.

If a future release loses a mutable executor acknowledgement, query only the
exact stable idempotency status. If no exact committed/not-sent proof exists,
record `outcome_unknown`; never issue a new Commit key or retry the external
write. Secret-handle cleanup, intent expiry and receipt reconciliation must
remain available while Runtime is disabled.

An authenticated pre-Commit cancellation must use the exact immutable intent
fingerprint and a stable cancellation ID. Cancel and Commit serialize on the
same PostgreSQL intent row: if Cancel wins, the prepared Attempt becomes
`canceled`, active handles are revoked and the executor count remains zero; if
Commit wins, never report rollback and continue receipt/`outcome_unknown`
reconciliation. Grant revocation is append-only and must also trigger immediate
in-process Secret byte zeroization through the coordinating Broker service.

## Child delegation operations boundary

Migration `087` stores immutable root/child authority, exact Parent Attempt
bindings, Parent reservations, settlements and Sandbox reap facts under
`agent_delegation_owner`. Only `agent_delegation_control` may read those tables
and call the exact `SECURITY DEFINER` functions; API, Orchestrator, Runner and
effect roles gain no delegation table DML. `neo-runnerd` still receives no
database, vault, object-store or Provider credential.

There is no public Child API, application startup worker or production
Backend-to-Runner Child launch path. Operators may run the delegation source
and disposable PostgreSQL commands above; they must not manually register root
authority or manufacture Child Runs in a live database. Reconciliation remains
an internal held seam: it discovers terminal/expired/reclaimed/Kill-Switched
Parents, fences Child leases and records exact reap work before invoking the
injected credential-free reaper. Failed reaps stay durable and must never be
cleared by restoring a lease or deleting authority rows.

## Cron scheduling operations boundary

Migration `088` stores immutable approved template revisions, exact cursors,
generation-fenced cursor/trigger claims, unique UTC occurrences, normal-Run
links and sanitized audits under `agent_cron_owner`. Only
`agent_cron_control` may read those tables and call the exact
`SECURITY DEFINER` functions; API, Orchestrator, Runner, effect and delegation
roles gain no Cron table DML. `neo-runnerd` receives no database, Cron, vault,
object-store or Provider credential.

There is no public Cron API, application startup worker, Redis wake loop,
frontend/Chat wiring or production Scheduler. Operators may run the Cron source
and disposable PostgreSQL commands above, but must not manufacture templates or
invoke enqueue functions in a live database. Runtime/Scheduler-off operation
may reconcile expired claims, terminalize exhausted retry facts, inspect audit
and run bounded retention cleanup. Pause and resume must use the narrow control
service; resume skips the paused window instead of backfilling it.

Migration `088` down fails with `AGENT_CRON_DOWN_DATA_EXISTS` while templates,
revisions, approvals/revocations, triggers or audits remain. Use delete
tombstones plus bounded prune before a deliberate clean rollback; never delete
protected runtime state or bypass immutable audit triggers to clear the guard.

## Draft learning operations boundary

Migration `089` stores immutable Draft specs, exact check receipts, append-only
human decisions, promotion links, generation-fenced cleanup work and sanitized
audits under `agent_learning_owner`. Only `agent_learning_control` may read the
tables and call the exact `SECURITY DEFINER` functions. API, Orchestrator,
Runner, effect, delegation and Cron roles gain no Draft table DML or Promote
execution.

There is no public Draft API/UI, Chat hook, startup worker, Redis wake loop,
Compose service or production isolation/evaluation adapter. Do not manufacture
Drafts or decisions in a live database. Operators may run the source and
disposable PostgreSQL gates above. With Learning disabled, claim
reconciliation, audit/read, object-before-row cleanup and bounded prune remain
available; Promote remains blocked.

Promotion writes the canonical package, SBOM and source-quarantine objects
before the atomic database transaction. A mismatched existing content-addressed
object is `DRAFT_OBJECT_DRIFT` and must not be overwritten. Orphaned objects
after a failed database transaction confer no authority and may be handled only
by a separately reviewed content-addressed orphan sweep.

Migration `089` down fails with `AGENT_LEARNING_DOWN_DATA_EXISTS` while any
Draft/check/decision/cleanup/audit fact or `learning` candidate remains.
Production rollback keeps `089` applied and disables Learning; clean down/up is
for disposable empty databases only.

## Agent Center and Shadow operations boundary

Migration `090` exposes sanitized Agent Center projections plus exact
`SECURITY DEFINER` functions under the NOLOGIN `agent_product_owner`. The API
role may read those projections and invoke only owned Artifact lookup, Run
cancel, approval, Cron lifecycle, human Draft review and Shadow control. It has
no worker table DML, lease/claim, effect Commit, delegation launch, Cron trigger
or learning-check authority. Artifact object keys remain server-only.

Agent Center is safe to deploy while Runtime is held: Run creation returns
`ISOLATION_UNAVAILABLE`, existing durable records remain readable, and Package
Skills stay separate from Assistants and MCP. Legacy text-Skill authority is
absent after G20.9. The Shadow policy
defaults off. Do not insert a synthetic adapter or call observation functions
from an ad-hoc production process; an adapter is test-only until the exact-host
isolation promotion is approved.

An operator enabling a later Shadow policy must bind the exact admitted
package/runtime fingerprints, revision, cohort basis points, start/expiry and
observation/error budgets. A user must opt in separately. Opt-out, policy drift,
restart boot epoch, budget breach, expiry, Kill Switch or fingerprint drift
fences new work. Diagnostics remain content-free and Shadow output never enters
Chat or admission/promotion decisions.

Migration `090` down fails with `AGENT_PRODUCT_DOWN_DATA_EXISTS` while any
cancellation, Artifact, policy, opt-in or observation authority remains.
Production rollback keeps `090` applied and leaves Shadow disabled. Clean
down/up is restricted to a verified-empty disposable database. Every older
Agent/MCP/Assistant/Skill migration drill must peel empty `090` before its own
guard and return to head `090`.

## Kill Switch operations

G20.2 persists and resolves the hierarchy through migration `084`; G20.3
migration `085` binds short-lived Runner authority to that current epoch and
stores expected Sandbox projection; G20.4 migration `086` binds every Prepare
and Commit to the same epoch; G20.5 migration `087` rechecks Parent/Child scope
at enqueue and launch and cascades affected Child leases; G20.6 migration `088`
checks global/scheduler/user/project/skill/admission/Secret scopes before every
Cron Run enqueue; G20.7 migration `089` rechecks source Run/package plus
applicable Kill Switches before Draft completion and Promote. No production
Runner, Broker, Child worker, Scheduler or Learning worker is started.
Switch removal remains a new inactive revision; cleanup/recovery/rebuild/
retention remain available. Exercise the boundaries with:

```bash
bash scripts/verify-agent-orchestrator.sh
bash scripts/verify-agent-orchestrator-postgres17.sh
bash scripts/verify-agent-runner-postgres17.sh
bash scripts/verify-agent-broker-postgres17.sh
bash scripts/verify-agent-delegation-postgres17.sh
bash scripts/verify-agent-cron-postgres17.sh
bash scripts/verify-agent-learning-postgres17.sh
```

These passes are durable control-plane evidence only, not rootless isolation or
production Runtime enablement evidence.

| Scope | Use | Expected effect |
| --- | --- | --- |
| global Runtime | unknown systemic risk | deny leases/effects; cancel or kill all Runs by mode |
| scheduler | runaway Cron | skip new triggers, retain templates/history |
| runner/host | compromised/unhealthy host | fence host leases and kill exact Sandboxes |
| source/admission | supply-chain incident | block fetch/admission; installed fingerprints separately revocable |
| Skill fingerprint | bad package/version | deny new use and optionally stop exact live Runs |
| Tool/action | executor incident | block matching Prepare/Commit without widening other Tools |
| Egress destination | endpoint/DNS incident | deny matching network requests immediately |
| Secret ref | credential incident | revoke Broker resolution/handles and rotate vault value |
| Project/user | abuse/data incident | deny scoped Runs and effects |
| Run | individual runaway | cancel/kill one lineage including Children |

Emergency procedure:

1. append the durable switch with actor/scope/mode/reason;
2. verify Backend observes the new epoch and stops relevant leases/Commit;
3. for `kill`, verify Runner kills the exact cgroups/process descendants and
   Scratch cleanup completes;
4. reconcile PostgreSQL nonterminal Runs and ambiguous effects;
5. retain evidence, fix forward, rerun acceptance gates;
6. removal is a new audited revision, never deletion of the incident record.

Do not stop Backend cleanup workers or delete rows/Sandboxes to make health look
green. Do not manually retry `outcome_unknown` writes.

## Isolation Acceptance runbook

The exact target-host command is `scripts/verify-agent-runner-host.sh`. Its output
must be machine-readable, content-free and fingerprint-bound. Minimum suites:

```text
probe/runtime identity
OCI config inspection
namespace/mount/socket/device escape negatives
Workspace/symlink/hardlink/path-race negatives
network/DNS/redirect/metadata negatives
Secret canary zero-leak
cgroup CPU/memory/PID/disk/output/wall exhaustion
cancel/kill/descendant/orphan/reboot cleanup
lease reclaim/stale message rejection
Prepare/Commit crash and acknowledgement-loss matrix (G20.4)
Child depth/registry/grant/budget narrowing (G20.5)
Runtime-off cleanup/retention
```

The current G20.3 host command performs the release-bound prerequisite probe
only; it must not report the later G20.4/G20.5 suites as executed. Production
promotion requires the complete list after the owning groups exist.

Evidence must identify target host class, kernel, runtime and storage/network
drivers, Runner image/binary, Runtime Bundle, seccomp, suite commit and result
hash. It excludes package/Workspace content, prompts, Tool bodies, secrets,
stdout/stderr and high-cardinality identities.

## Backup and restore

The durable backup set eventually includes:

- PostgreSQL admissions, installs, Grants, snapshots, Run/Step/Attempt/events,
  approvals, Cron revisions, Draft decisions, Agent product cancellation/
  Shadow policy/opt-in/observation facts, cleanup queue and Kill Switches;
- object-store Skill packages, Runtime Bundles, SBOMs, Workspace snapshots and
  published Artifacts;
- a manifest binding DB backup, object mirror, migration head and checksums.

Restore occurs with Runtime/Scheduler disabled. Validate objects and rebuild
projections before opening Backend. Reconciliation kills all pre-restore live
Sandboxes because their leases/nonces cannot be trusted across the restore
boundary. Secret vault/mTLS backups follow their own encrypted rotation
procedure and are not embedded in Agent artifacts.

## G20.10 production operations policy

`config/agent-runner/production-policy.json` freezes the first single-server
operations defaults. It is an acceptance-policy input, not an activation flag
and not evidence that a worker exists. A change to any value creates a reviewed
policy revision, changes its SHA-256, invalidates old closure records and repeats
the affected canary/acceptance checks.

Capacity defaults:

| Boundary | Default | Saturation behavior |
| --- | ---: | --- |
| concurrent root Runs | 2 | queue, then deny when queue is full |
| concurrent Sandboxes | 4 | no launch until exact capacity is free |
| queue depth | 32 | stable capacity denial; never another executor |
| concurrent production canaries | 1 | serialize canary evidence |

Frozen budget defaults:

| Run class | wall | model tokens | Tool calls | Artifact bytes |
| --- | ---: | ---: | ---: | ---: |
| root | 300 seconds | 20,000 | 32 | 16 MiB |
| Child | 120 seconds | 8,000 | 8 | 4 MiB |
| Cron | 120 seconds | 8,000 | 8 | 4 MiB |

These are ceilings only when the frozen Grant is not narrower. Child and Cron
budgets remain strict subsets; no default expands a Run snapshot. The first
canary window is at most 20 `synthetic|read_only` Runs in 30 minutes, one at a
time, with no Egress, no Secrets, no mutable action, at most one ordinary error
and exactly zero `outcome_unknown` results.

## Metrics and alerts

Required metrics are listed in the policy and use only the label allowlist
`operation`, `outcome`, `reason_code`, `runtime_class`, `scope_class` and
`switch_mode`. Do not use Run/Attempt/user/Skill IDs, fingerprints, URL/object
keys or any content as a metric label. The closure evidence hashes the scrape
and alert-routing drill; it does not embed a scrape body.

| Alert | Severity | Required response |
| --- | --- | --- |
| Runtime enabled while exact readiness is false | page immediately | append global `deny_new`; disable activation; inspect release binding |
| Secret canary leak | page immediately | append global/Secret `kill`; revoke handles; rotate value and mTLS as applicable |
| unresolved `outcome_unknown` | page immediately | freeze the exact Tool/action or Run; execute the no-retry workflow below |
| Kill Switch enforcement failure | page immediately | stop new authority at the broadest safe upstream boundary; preserve evidence |
| orphan after two reconciliation passes | page within 5 minutes | runner/global `kill`; exact cgroup/process inventory and reap |
| queue at 24+ for 15 minutes | ticket | reduce cohort or add an independently accepted Runner; never widen budgets |
| cleanup backlog at 100+ for 30 minutes | ticket | keep Runtime-off cleanup running; repair object/reaper dependency |

The page path must be exercised end to end before promotion. An alert rule file
that merely parses is not evidence; the receiver acknowledgement hash and
bounded timestamps belong in the external evidence file.

## Retention and cleanup

| Authority | Minimum retention / eligibility |
| --- | --- |
| terminal Run/Step/Attempt/events/snapshot | 30 days, and only after dependent Artifact/effect cleanup |
| completed Runner request/Sandbox projection | 7 days after exact terminal cleanup |
| temporary canary Artifact and rejected/promoted Draft object | 7 days; object before row |
| Cron trigger history | 30 days after linked Run is terminal and eligible |
| immutable Cron/Draft/Kill-Switch incident audit | 365 days |
| content-free promotion record | 400 days |
| unresolved `outcome_unknown`, object drift, failed reap/cleanup | until explicit operator resolution; never age-only prune |

Run prune/reconcile in batches of at most 100. Before calling terminal-Run
retention, the operator must prove there is no unresolved `outcome_unknown`
inside the candidate set; migration `084`'s generic terminal prune does not
infer an external incident resolution. Cleanup remains active while Runtime,
Scheduler, Learning and Shadow execution are off. Never delete a row/audit to
silence a backlog. Delete/quarantine exact object bytes and reap processes/
Scratch first, then acknowledge or prune the durable projection.

## On-call incident workflows

### Isolation drift, runaway Run or orphan residue

1. Append the narrowest safe durable `deny_new`, `cancel` or `kill` switch. Use
   global `kill` only when the affected scope cannot be bounded safely.
2. Verify the new epoch fences leases and Commit before touching host state.
3. Compare PostgreSQL expected inventory with `neo-runnerd` inventory. Kill only
   exact owned cgroups/process descendants; remove exact Scratch/staging.
4. Run Child-first reap and Runner reconciliation twice. Any remaining orphan
   is still an incident, not a row to delete.
5. Repair the immutable release/host cause, repeat the full acceptance suite,
   then append a new inactive switch revision. Never delete the active-history
   record or re-enable a stale lease.

### `outcome_unknown` operator workflow

1. Append a Tool/action or Run `deny_new` switch and preserve the exact intent
   fingerprint and stable idempotency key through the authorized content-free
   view. Never copy arguments/results into the incident record.
2. Query only the executor's non-mutating exact-idempotency status. Never send a
   new Commit, change the key, replay arguments or claim cancellation rollback.
3. If an exact committed receipt exists, record its fingerprint. If the exact
   status proves rejected/not-sent, record that status fingerprint. If neither
   proof exists, keep the durable state terminal `outcome_unknown`.
4. Append a content-free operator resolution in the incident authority and bind
   its SHA-256 to the production-closure check. Do not rewrite the original
   Run/Attempt/effect receipt.
5. Retention remains blocked for the affected authority until review closes the
   incident and verifies downstream business state separately.

### Credential, mTLS and Runtime Bundle rotation

- **Secret credential:** append the Secret-ref switch/revocation, zeroize active
  in-memory handles, rotate in the vault, issue only new short-lived handles and
  run a zero-leak canary before appending the inactive switch revision.
- **Runner mTLS:** pause new leases, reconcile/drain, add the new CA/client/server
  identities as an overlap set, restart one side at a time, prove TLS 1.3 and
  replay fencing, then remove the old root. Keys remain mode `0600` files and
  never enter the closure record.
- **Runtime Bundle:** publish a new immutable fingerprint and repeat isolation,
  clean-copy, restart and bounded canary checks. Existing Runs remain bound to
  the old snapshot until terminal; never replace bytes under an old digest.

Each rotation emits only time, result class and evidence SHA-256. Rotation that
changes the closure release tuple invalidates the old promotion record.

## Disaster recovery and forward repair

1. Append global `deny_new`; stop Runtime/Scheduler/Learning/Shadow workers and
   keep Backend unopened for restored authority.
2. Verify one paired PostgreSQL/MinIO set manifest and restore both halves into
   the isolated target. Follow `backup-restore.md`, including the latest
   encrypted Memory deletion package replay before opening Backend.
3. Apply the exact release migrations and require head `090`. Rehash every
   referenced Package/Runtime/SBOM/Workspace/Artifact sample from the restored
   set.
4. Treat all pre-restore leases, Runner nonces and Sandboxes as untrusted. Kill
   and reconcile them; rebuild projections and reconcile committing effects,
   Children, Cron claims, Draft cleanup and Artifact cleanup with execution off.
5. Repeat readiness, clean restart, host reboot and read-only canary. Exercise
   both previous-image all-path rollback inside its declared window and a
   forward-fix return to the current immutable release.
6. Open the Backend only after deletion/reconciliation checks are zero or
   explicitly held. A partial database-only/object-only restore cannot promote.

## Production closure evidence gate

The schema is
`docs/contracts/schemas/neo-agent-production-closure.schema.json`. Keep actual
records outside Git and outside application/object-store runtime namespaces.
They contain fingerprints, times, low-cardinality result codes and zero-counts
only. The record binds exactly 16 live checks to one Git commit, migration head
`090`, Runner manifest/binary, Runtime Bundle, deployment and operations-policy
fingerprint.

Run the offline contract self-test:

```bash
bash scripts/verify-agent-production-closure.sh
```

Evaluate a separately captured production record without mutation:

```bash
bash scripts/verify-agent-production-closure.sh \
  --record /secure/operator-evidence/agent-production-closure.json
```

Exit `0` plus `PROMOTION_READY` is necessary but not self-activating. Exit `3`
is a valid held decision; exit `2` means malformed/drifted evidence. The
committed template intentionally returns exit `3`, `PROMOTION_HELD` and
`ISOLATION_UNAVAILABLE`. A template can never promote even if all checks are
edited to `passed`.

After the live record has been hashed and reviewed, delete temporary canary
Runs, Draft quarantine, published canary Artifacts, raw Run evidence, orphan
Sandboxes and Scratch through their bounded authoritative cleanup paths. Every
closure cleanup count must be zero while the content-free closure/incident
record remains retained. Do not delete user Runs, installed packages, immutable
audits or unresolved incidents merely because they were observed during the
promotion window.

## Legacy Skill cutover and rollback

G20.9 is a one-way application/data cutover, not migration `091`. Execute it in
this order:

1. While the G20.8 image is still running, freeze legacy Skill edits and create
   its explicit raw browser backup/inventory. Capture the exact manifest
   fingerprint without logging Skill bodies.
2. Disable writes for the maintenance window. Create a full PostgreSQL backup,
   record `sha256sum` as `sha256:<64 lowercase hex>`, and count:

   ```sql
   SELECT count(*) FROM conversations WHERE metadata ? 'activeSkills';
   ```

3. Run the SQL with no variables first. This is always dry-run and rolls back:

   ```bash
   psql "$DATABASE_URL" --file scripts/cutover-legacy-skills.sql
   ```

4. Apply only when the count and backup fingerprint match:

   ```bash
   psql "$DATABASE_URL" \
     --variable=cutover_apply=true \
     --variable=expected_count="$EXPECTED_COUNT" \
     --variable=backup_fingerprint="sha256:$BACKUP_SHA256" \
     --file scripts/cutover-legacy-skills.sql
   ```

   The transaction takes a `SHARE ROW EXCLUSIVE` lock, removes only
   `conversations.metadata.activeSkills`, checks the updated count and requires
   zero remaining keys. It does not rewrite message/content/other metadata or
   advance schema head beyond `090`.
5. Deploy G20.9. Browser persistence version `7` strips the eight retired
   settings fields plus Session/Workspace selections from localStorage and
   IndexedDB, writes its marker last, and compensates partial failure. Reload
   twice and confirm the migration is idempotent and no `partialize` path
   resurrects state.
6. Verify old message arrays render only “旧版技能已退役”; no legacy identity or
   definition remains. Run both G20.9 gates below.

```bash
bash scripts/verify-agent-legacy-cutover.sh
bash scripts/verify-agent-legacy-cutover-postgres17.sh
```

Assistant, MCP, Chat content/tree, Conversation fields other than the retired
key, files, Knowledge and Memory remain unchanged. Admitted Package Skills are
the sole eligible Skill domain after source cutover, but production execution
stays held at `ISOLATION_UNAVAILABLE`; never add browser/API/rootful fallback or
dual execution.

Rollback uses a pre-cutover backup plus previous application images only inside
the declared window and only as an all-path rollback. Once new Runtime writes or
new Skill installations are accepted, prefer forward repair; never rehydrate
old Skill definitions from history labels.

## Safe diagnostics

Allowed: service/probe readiness, counts by state/error class, Run/Step/Attempt
IDs, lease generation, fingerprints, bounded duration/bytes/calls/tokens and
Kill Switch scope/mode.

Never log or paste: Skill/Workspace/prompt content, Tool arguments/results,
stdout/stderr, Artifact bytes, host paths, subuid private mapping details beyond
the probe classification, mTLS keys, Secret refs/handles/values, Provider
payloads, bearer/lease tokens or custom URL paths.

## Phase 0 command

From `mm-chat/`:

```bash
bash scripts/verify-agent-runtime-phase0.sh
```

This gate also checks the G20.8 facade/Shadow, G20.9 hard-retirement, G21.0
control-only wiring and G21.1 Root canary signatures. A pass means the design
and held product artifacts are internally consistent; it does not mean this
host or production has a usable rootless Runtime.
