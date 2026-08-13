# Neo Agent Runtime Operations

Status: G20.1 no-execute Skill supply and G20.2 durable Orchestrator authority
are implemented. Do not install a Runtime, start `neo-runnerd`, enable Agent
execution or delete legacy Skills from this document alone.

## Default state

All future switches default off:

```text
AGENT_RUNTIME_ENABLED=false
AGENT_SCHEDULER_ENABLED=false
AGENT_SKILL_INSTALL_ENABLED=false
AGENT_LEARNING_ENABLED=false
AGENT_DELEGATION_ENABLED=false
AGENT_RUNNER_URL=
```

G20.1/G20.2 add no environment variables or Compose service. These names reserve
the intended operational boundary; later implementation must add them through
the normal preflight/example-env/Compose/documentation gates.

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
  + per-Run Scratch
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

Current development-host observation (2026-08-13): cgroup v2, user namespace
quota and subuid/subgid exist; Docker 29.7.2 is available; Podman, crun, runc,
rootlesskit, slirp4netns, pasta, fuse-overlayfs and bwrap commands are absent.
This is a planning fact, not a failed production deployment. Phase 0 does not
install or change the host.

## Service account and filesystem boundary

Future deployment must use a dedicated non-login `neo-runner` identity, not the
Go API or database user. Required writable state is minimal and separated:

```text
runtime cache: immutable-digest keyed; no source checkout
run staging: one mode-0700 directory per Attempt; quota and owner checked
scratch: one mount per Run; removed on all terminal/orphan paths
probe evidence: bounded metadata only
mTLS key: mode-0600 secret file, not env/Git/image
```

Package candidates are fetched/admitted by the Control Plane into quarantine.
Runner consumes only admitted immutable object coordinates and verifies their
fingerprints again. Runner must never `git clone`, resolve floating dependencies
or run package installers from a live source at Run time.

## mTLS control plane

- bind only a private Unix socket or dedicated private interface;
- separate CA/service identities for API and Runner;
- mode-0600 private keys from mounted secret files;
- rotate by overlapping trust roots, restarting one side at a time, then
  removing the old root after reconciliation;
- version negotiation and `probe` must pass before readiness;
- nonce/request replay state is durable or otherwise cannot be reset into an
  acceptance window by ordinary process restart;
- logs include request/Run/Attempt IDs and error class only.

No public reverse-proxy route exposes Runner RPC. Frontend never receives its
URL, certificate, nonce, lease token, package coordinate or Secret handle.

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

## Kill Switch operations

G20.2 persists and resolves the hierarchy through migration `084`. It fences
enqueue, leases, heartbeat and nonterminal Attempt progress, but it deliberately
has no Runner process to cancel or kill yet. Switch removal is a new inactive
revision; cleanup/recovery/rebuild/retention remain available. Exercise the
database-only boundary with:

```bash
bash scripts/verify-agent-orchestrator.sh
bash scripts/verify-agent-orchestrator-postgres17.sh
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

The exact future command is owned by the runner implementation group. Its output
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
Prepare/Commit crash and acknowledgement-loss matrix
Child depth/registry/grant/budget narrowing
Runtime-off cleanup/retention
```

Evidence must identify target host class, kernel, runtime and storage/network
drivers, Runner image/binary, Runtime Bundle, seccomp, suite commit and result
hash. It excludes package/Workspace content, prompts, Tool bodies, secrets,
stdout/stderr and high-cardinality identities.

## Backup and restore

The durable backup set eventually includes:

- PostgreSQL admissions, installs, Grants, snapshots, Run/Step/Attempt/events,
  approvals, Cron revisions, Draft decisions, cleanup queue and Kill Switches;
- object-store Skill packages, Runtime Bundles, SBOMs, Workspace snapshots and
  published Artifacts;
- a manifest binding DB backup, object mirror, migration head and checksums.

Restore occurs with Runtime/Scheduler disabled. Validate objects and rebuild
projections before opening Backend. Reconciliation kills all pre-restore live
Sandboxes because their leases/nonces cannot be trusted across the restore
boundary. Secret vault/mTLS backups follow their own encrypted rotation
procedure and are not embedded in Agent artifacts.

## Legacy Skill cutover and rollback

Cutover inventory includes browser persistence version, all local settings keys,
Conversation/Workspace `activeSkills`, text Skill catalogs/custom definitions,
selection UI/context assembly and historical `skillInvocations` projection.

Final switch rules:

- hard delete legacy definitions/state; do not convert them to packages;
- run a bounded browser storage migration that purges obsolete keys;
- clear server Conversation/Workspace selection references;
- keep message history, projecting only “旧版技能已退役” as a fact;
- new Runtime is the only executable Skill path after cutover;
- no dual-write/dual-execute fallback.

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

This is an offline contract check only. A pass means the design artifacts are
internally consistent; it does not mean this host or production has a usable
rootless Runtime.
