# Agent Runtime Local Test-Host Activation

## Goal

Prepare the current Ubuntu 22.04 WSL2 machine to run one real, tightly bounded
Agent Skill smoke inside the project's rootless OCI isolation path, while
keeping every production activation and promotion gate closed.

In user terms: install the safe execution box on this test machine, run one
harmless real Skill in it, and leave a clear path for later page wiring.

## Requirements

- Add a separate `local_test` host profile. It must not modify, replace, relax,
  or reinterpret the existing production release manifest, host probe,
  activation records, closure evaluator, or promotion evaluator.
- Support the observed host only: Ubuntu 22.04, WSL2, `amd64`, cgroup v2, and a
  non-root operator with a non-overlapping 65,536-entry subordinate UID/GID
  range. Unsupported hosts fail closed with a short reason code.
- Provide a read-only preflight that explains the next action in plain
  language and performs no privileged mutation.
- Provide one explicit privileged bootstrap command for the operator to run in
  their own terminal. Automation must never ask for, read, pipe, log, or store
  a sudo password.
- The privileged bootstrap must preserve existing `/etc/wsl.conf` sections,
  enable systemd idempotently, install only an allowlisted set of Ubuntu
  build/rootless prerequisites, and record enough non-secret state for guarded
  rollback.
- Treat the required WSL shutdown/restart as an explicit boundary. Never start
  it automatically from the development session.
- Pin and verify the complete downloaded toolchain before use: Go 1.25.9,
  Podman 6.1.0 source, crun 1.29.1 with systemd support, and conmon 2.2.1.
  Build/install as the normal user into a dedicated local-test root.
- Generate dedicated rootless Podman configuration and storage paths without
  modifying global production Runner paths or the checked-in unapproved
  production template.
- Verify systemd user availability, delegated cgroup v2 CPU/memory/PID
  controllers, user namespaces, subordinate IDs, setuid mapping helpers,
  pinned binary hashes/versions, rootless Podman, crun, seccomp, overlay
  storage, and `network=none` before smoke execution.
- Run exactly one deterministic, disposable, no-network/no-secret/no-user-data
  Skill workload with a read-only root filesystem, nonzero UID/GID, empty
  capabilities, `no-new-privileges`, the reviewed seccomp file, CPU/memory/PID/
  wall/output/scratch bounds, a read-only synthetic Workspace, and complete
  cleanup.
- Emit content-free local-test evidence with an explicit `local_test` class.
  It must be structurally ineligible for `ACTIVATION_READY`,
  `PROMOTION_READY`, production activation, or production closure.
- Provide idempotent status and guarded rollback commands. Rollback must never
  touch `mm-chat/.env.single-server`, `mm-chat/data/`, `mm-chat/secrets/`, or
  `mm-chat/backup/`, must not run `apt autoremove`, and must not remove packages
  that predated the bootstrap.
- Keep user-facing instructions short and plain: check, run one sudo command,
  restart WSL, install the local toolchain, run the smoke, or roll back.

## Acceptance Criteria

- [ ] The read-only preflight on the current baseline returns a stable
  `RESTART_REQUIRED` or `SYSTEM_BOOTSTRAP_REQUIRED` result and makes no host
  changes.
- [ ] Script/source tests prove strict lock parsing, hash drift rejection,
  unsupported-host rejection, sudo-password absence, idempotent `wsl.conf`
  editing, package allowlisting, protected-path exclusion, and guarded
  rollback.
- [ ] Existing `bash mm-chat/scripts/verify-agent-runner-host.sh` still returns
  `ISOLATION_UNAVAILABLE` against the checked-in template on this development
  tree.
- [ ] After the operator performs the one privileged step and WSL restart, the
  exact pinned local-test toolchain installs without rootful Docker execution.
- [ ] A real bounded local-test Skill smoke passes and leaves zero managed
  containers, scratch files, or broker files.
- [ ] The emitted report says `local_test`, contains no prompts, Workspace
  contents, secrets, credentials, environment values, or production-ready
  status, and is rejected by production evidence paths.
- [ ] Focused script tests, Backend tests/vet for any changed Go package, the
  Agent Runner gates, and the standalone quality gate pass as applicable.
- [ ] Operations docs explain activation, status, restart, and rollback in
  plain operator language.

## Definition of Done

- Tests cover positive, negative, replay/idempotency, cleanup, and rollback
  behavior without requiring sudo in CI.
- Formatting, lint, type-check, focused tests, and relevant Agent Runtime gates
  are green.
- Operations contracts and tracking notes record the local-test/production
  boundary.
- Changes are committed without amend or push; task archive and journal remain
  separate commits.

## Technical Approach

Use a native WSL local-test profile. A small privileged bootstrap handles only
systemd configuration and allowlisted Ubuntu prerequisites. A separate
unprivileged installer verifies a checked-in upstream lock, builds Podman from
the exact pinned source archive with its vendored Go dependencies, and installs
all toolchain artifacts into a dedicated local-test root. A local verifier then
proves the actual rootless isolation properties and runs one synthetic
disposable Skill.

Production artifacts remain untouched. Local evidence uses a different schema
and class and is never accepted as an approved Runner release, production
activation, closure, or promotion record.

## Decision (ADR-lite)

**Context**: The current machine is the user's test machine. Ubuntu 22.04's
Podman/crun packages are too old, systemd is disabled, and Docker fallback is
forbidden by the existing isolation contract.

**Decision**: Enable systemd on the existing WSL distro, install only host
prerequisites with an operator-run sudo command, build the exact pinned Podman
engine as the normal user, use the exact upstream crun/conmon binaries, and
keep all results explicitly local-test-only.

**Consequences**: One interactive sudo action and one WSL restart are required.
The current user, rather than a dedicated production Runner account, owns the
local rootless engine, so the result is valid for local feature testing only.
No production readiness can be inferred. A later task must connect this proven
local runtime to the page-facing Agent workflow.

## Out of Scope

- Production host provisioning, production credentials, production evidence,
  `ACTIVATION_READY`, `PROMOTION_READY`, or changing the production gate.
- Rootful Docker/Podman, privileged containers, networked Skills, real user
  Projects, real prompts, Provider access, MCP calls, Egress, or Secrets.
- A separate VM, VPS, physical machine, Podman machine, or WSL distro.
- Public/API/Chat Agent execution and final page wiring; this slice proves the
  local host and one bounded Skill first.
- Reading or modifying protected runtime state.

## Research References

- [`research/wsl-rootless-runtime.md`](research/wsl-rootless-runtime.md) — host
  facts, official WSL/Podman requirements, pinned upstream inputs, alternatives,
  and rollback constraints.

## Technical Notes

- Relevant implementation: `mm-chat/backend/internal/agentrunner`,
  `mm-chat/backend/cmd/neo-runnerd`, `mm-chat/scripts/verify-agent-runner-host.sh`,
  `mm-chat/config/agent-runner`, and `mm-chat/deploy/agent-runner`.
- Relevant contracts: `.trellis/spec/backend/agent-runtime.md` and
  `.trellis/spec/operations/agent-runtime.md`.
- The user selected the recommended path for all ordinary choices and asked
  the developer to proceed and commit without repeated confirmation.
