# Neo Runtime Fit Research

## Current Runtime Truth

Host probe on 2026-08-13:

```text
kernel: Linux 6.18.0-1018-oem-WSL2
cgroup filesystem: cgroup2fs
user.max_user_namespaces: 63475
subuid: mumu:100000:65536
subgid: mumu:100000:65536
Docker client: 29.7.2
missing commands: podman, crun, runc, rootlesskit, slirp4netns, pasta,
                  fuse-overlayfs, bwrap
```

Docker availability does not prove a rootless OCI data plane. Phase 0 therefore
records the required probe but does not install packages or claim isolation
acceptance.

## Existing Neo Seams Worth Reusing

- Go/PostgreSQL already owns authenticated server state and durable chat runs.
- G19 has provider-native Tool continuation and durable sanitized process trace.
- MCP has immutable run snapshots, server-owned grants, Tool classification,
  `outcome_unknown`, result artifact storage and transport kill switches.
- Provider vault patterns already separate encrypted secret storage from public
  DTOs and require explicit activation/test.
- `codejobs` already fails closed at `CODE_EXECUTION_UNAVAILABLE` and has a
  documented sandbox gate.
- MinIO/object storage is already behind Backend authorization; Sandbox must use
  a broker rather than direct credentials.

These patterns should be extended, not duplicated. Agent Runtime still needs a
separate runner/data-plane protocol because MCP Runner executes approved Tool
servers, not arbitrary per-Run Skill workloads.

## Current Legacy Skill Cutover Inventory

Frontend browser state currently includes:

```text
settingsStore.installedSkills
settingsStore.customSkills
settingsStore.activeSkillIds
settingsStore.skillAutoSelect
Conversation.config.activeSkills
Workspace.activeSkills
Message.skillInvocations
```

`ChatApp.tsx`, `MessageInput.tsx`, `effectiveChatContext.ts` and
`skillService.ts` still select Skills and build text context in the browser.
The persistence version is currently `6`. A final cutover must delete active
configuration and code paths, bump/migrate storage to purge obsolete state, and
project old message invocations only as a read-only retired fact. It must not
delete history itself.

## Recommended Topology

```text
Go API / Orchestrator
  - identity, admission, snapshot, state machine, approvals, audit
  - PostgreSQL durable authority
  - object storage package/runtime/artifact authority
             |
             | mTLS + version + nonce + replay fence
             v
host non-root neo-runnerd
  - runtime capability probe
  - rootless OCI lifecycle
  - event/output backpressure
             |
             v
per-Run Sandbox
  - immutable rootfs/package
  - project snapshot + scratch
  - broker-only artifacts/egress/secrets/tools
```

## Main Risks and Required Controls

| Risk | Required Phase 0 contract |
| --- | --- |
| untrusted Skill supply chain | canonical fingerprint, SBOM, admission, archive limits, immutable bundle |
| prompt-driven privilege expansion | server grant intersection; Skill/model output never grants authority |
| stale lease side effects | lease generation on every RPC/Commit; old Attempt hard rejection |
| ambiguous external writes | Prepare/Commit, idempotency, `outcome_unknown`, no blind retry |
| sandbox escape/host theft | rootless OCI, seccomp/cgroup/no caps/no socket/no host bind, acceptance suite |
| secret exfiltration | broker handles, action-bound short TTL, no env/prompt/artifact/log persistence |
| DNS/HTTP bypass | brokered egress, resolution pinning, redirect recheck, metadata/link-local denial |
| recursive agents | fixed depth schema + child registry physical exclusion + RPC admission test |
| self-modifying production | Draft quarantine, human Promote, new fingerprint, snapshot immutability |
| destructive legacy cutover | backup + inventory + switch gate + forward rollback window + history label |

## Phase Boundary

Phase 0 may add documents, schemas, fixtures and an offline verifier only. It
must not change frontend/backend runtime behavior, live config, database schema,
runtime data or host dependencies.
