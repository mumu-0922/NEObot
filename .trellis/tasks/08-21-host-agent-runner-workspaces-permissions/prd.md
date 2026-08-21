# Harness-compatible host runner workspaces and permissions

## Goal

Bring neo-chat close to the DeepSeek Harness local-agent experience: a user can
register and select arbitrary Windows or WSL project directories, bind a chat
session to that project, execute Agent tools in the real project working
directory, and switch durable `Read Only`, `Workspace Write`, and `Full access`
permission presets whose enforcement is real rather than presentation-only.

## What I already know

* The user explicitly chose complete Harness-style behavior instead of a
  Docker-only pre-mounted-directory MVP.
* The browser, Docker Backend, and host execution plane run on one authorized,
  single-user Windows/WSL machine.
* Docker Backend mounts only one static project today; Docker cannot add an
  arbitrary host bind to an already-running container.
* DeepSeek Harness runs its Host execution and directory-picker capabilities
  on the machine that owns the filesystem, persists canonical workspace paths,
  binds session `cwd` to the selected workspace, and enforces permission state
  at filesystem/shell capability boundaries.
* neo-chat already has durable Chat Agent events, durable approval requests,
  transcript streaming/replay, local file/terminal tools, and a frontend
  Workspace grouping surface that can be reused visually.
* neo-chat's current frontend `Workspace` is a prompt/files grouping object,
  not a filesystem project or Backend execution authority.
* User preference: automatically commit and build/deploy finished slices; do
  not push. Run focused tests unless a full gate is genuinely necessary.

## Assumptions (temporary)

* A WSL-native ordinary-user Host Runner is the primary execution authority;
  it can access WSL paths directly and Windows drives through `/mnt/<drive>`.
* A Windows folder chooser can be invoked from the WSL Host Runner and selected
  Windows paths can be normalized through `wslpath`.
* Existing non-empty conversations keep their historical execution behavior;
  real project binding is immutable after the first durable turn.
* The initial production target is single-user local deployment. Remote
  multi-user runner tenancy is not part of the first release.

## Open Questions

* None currently.

## Requirements (evolving)

* Add a host-side Agent Runner outside Docker, running as the ordinary local
  user and authenticated to the Docker Backend over a local bounded channel.
* Provide one deployment wrapper that builds/starts or reconnects the Host
  Runner before starting the Docker application, without requiring WSL
  systemd. Re-running the wrapper must be idempotent.
* Add a durable Backend filesystem-workspace domain with stable id, runner id,
  canonical execution path, display path, platform/path kind, title, and
  timestamps.
* Support native Windows folder selection plus WSL filesystem browsing without
  requiring `.env` edits, Docker bind changes, or Backend recreation per
  project.
* Bind each Agent conversation to one durable workspace and immutable working
  directory once work begins.
* Execute file and terminal tools on the Host Runner and preserve the existing
  transcript, approval, cancellation, timeout, output-bound, and verification
  contracts.
* Add durable per-conversation permission presets: `read-only`,
  `workspace-write`, and `danger-full-access`.
* Enforce permission presets at the Host Runner boundary; never advertise a
  mode whose effects are not actually enforced.
* Require explicit risk confirmation before enabling Full access.
* Keep legacy conversations and the current prompt/files Workspace feature
  compatible during migration.
* Upgrade the existing visible Workspace concept in place: preserve its id,
  name, prompt, files, color, search/reasoning settings, and conversation
  membership while adding durable Runner/path binding. A legacy Workspace
  without a project path remains visible as `unbound` and can be bound through
  the directory flow without recreating or losing its contents.
* Provide fail-closed behavior when the runner disconnects, restarts, changes
  identity, or can no longer resolve the selected workspace.

## Acceptance Criteria (evolving)

* [ ] A user can register `/home/...`, `/mnt/d/...`, or a Windows `D:\\...`
      selection and receive one canonical durable workspace record.
* [ ] Re-registering path aliases for the same directory resolves the same
      workspace instead of creating duplicates.
* [ ] A new Agent conversation opened in a workspace runs file and terminal
      tools with that workspace as its actual `cwd`.
* [ ] Project selection requires no Backend container rebuild or bind-mount
      edit.
* [ ] Read Only prevents durable project mutations from file and terminal
      capabilities.
* [ ] Workspace Write permits writes inside the selected workspace and denies
      or explicitly escalates writes outside it.
* [ ] Full access requires UI acknowledgement and is bounded by the Host
      Runner process user's operating-system authority.
* [ ] Permission/workspace state survives browser refresh and Backend restart.
* [ ] Runner loss produces a durable, understandable failed/interrupted Tool
      state and never silently falls back to Docker local execution.
* [ ] Historical conversations without a Host Runner workspace still load.
* [ ] Existing prompt/files Workspaces migrate without losing settings,
      attachments, or conversation membership; an unbound Workspace can be
      bound exactly once to a canonical project directory.
* [ ] Focused Backend, Runner, Frontend, migration, and security tests pass.

## Definition of Done

* Tests added or updated at each affected layer.
* Focused lint, type-check, unit/integration tests, and production builds pass.
* Configuration, deployment, security, storage, protocol, and rollback docs are
  updated.
* Each independently deployable slice has a rollback path and is committed
  automatically without pushing.

## Out of Scope (explicit)

* Selecting the filesystem of a different remote browser device.
* Multi-tenant shared runners or arbitrary Internet-exposed runner admission.
* Silently granting the Backend container new host mounts.
* Deleting user project directories when a workspace registration is deleted.
* Claiming complete host isolation before the permission enforcement probes
  pass on the target Windows/WSL runtime.

## Technical Notes

* DeepSeek Harness reference commit inspected: `141eb6fef83422698aef7a981029e843e8161534`.
* Research references will live under `research/` and record the source paths,
  enforcement model, and neo-chat compatibility gaps.
* Likely neo-chat areas: `mm-chat/backend/internal/chat/`, a new Host Runner
  command/package, Backend migrations/repository/API, `mm-chat/frontend/src/`
  chat shell/sidebar/composer, Compose socket/token wiring, and deployment
  documentation.

## Technical Approach

### Host transport and identity

* Add a non-root `cmd/agent-host` binary to the existing Go module and keep the
  protocol implementation in dedicated internal packages rather than coupling
  it directly to Chat handlers.
* Serve bounded HTTP/1.1 over a permission-restricted Unix socket in a new
  runtime-state directory. Mount that directory into the Backend container;
  do not expose a TCP listener.
* Use an independent generated bearer token, strict JSON decoding, request and
  response byte limits, deadlines, protocol versions, stable error codes, and
  a durable Runner id. Reuse conventions from the MCP Runner, not its token or
  execution authority.
* Stream execution frames as bounded NDJSON. Closing/cancelling the Backend
  request cancels the Host context and kills the complete process group.
* Never retry an execution whose outcome is unknown. Runner restart turns
  in-flight operations into the existing durable interrupted/unknown outcome.

### Workspace authority

* The Host Runner is the only component that canonicalizes or probes Host
  paths. It returns canonical execution path, stable display path, path kind,
  and a directory identity/fingerprint to the Backend.
* The Backend owns user authorization, durable Workspace records, conversation
  binding, ordering, and legacy Workspace import.
* Windows chooser results are converted with the fixed Host helper and
  `wslpath`; directory browsing reads the WSL Host filesystem rather than the
  browser device.
* Workspace identity is unique per user, Runner, and canonical directory.
  Deleting registration never deletes the directory.

### Execution and permissions

* Move local file/terminal/job execution behind a Runner client while keeping
  Backend-owned Tool registration, approval, events, transcript, completion
  verification, and policy accounting.
* Resolve the access preset at the Backend for every execution and send the
  exact mode and workspace identity to the Runner. The Runner independently
  verifies that the request matches its canonical workspace record.
* Enforce read-only/workspace-write shell effects with a bundled/probed Linux
  filesystem sandbox. File Tools also use descriptor-relative containment and
  symlink defenses. Full access skips the file boundary but never elevates the
  operating-system user.
* Capability probes advertise only modes actually enforced on the current WSL
  kernel and filesystem (including DrvFS). Unsupported modes fail closed and
  stay unavailable in the UI.

### Rollout

* Introduce feature flags and a runner-health projection before routing any
  existing execution to the Host.
* Migrate and bind Workspace data before enabling Host execution.
* Route exact canary users/conversations first. No automatic fallback to the
  Docker local executor after a conversation is Host-bound.
* Keep the current Docker local runtime as a rollback path only for unbound
  legacy conversations until Host execution reaches parity.

## Implementation Plan (small committed slices)

1. **Protocol and Host foundation** — completed 2026-08-21: versioned
   contracts, Unix-socket Host server/client, identity/capability probes,
   token/socket lifecycle wrapper, focused protocol and security tests. The
   deployed dark Runner advertises no execution or permission modes and does
   not alter current Compose routing.
2. **Durable Workspace domain** — completed 2026-08-21: migration `102`
   extends the existing Workspace registry in place, preserves browser
   settings, adds CAS repository/service/API and immutable Conversation
   execution snapshots, and passes runtime-role PostgreSQL 17 down/re-up and
   ownership/locking tests. The deployed bind route remains fail closed until
   the next socket-wiring slice supplies the pinned Runner resolver.
3. **Workspace UI convergence** — completed 2026-08-21: Backend socket and
   Docker-secret wiring, Host status/browse/native-picker protocol, server-
   authoritative Workspace UI/state convergence, Conversation grouping round
   trip, focused tests/build, targeted deployment, and live Runner-loss smoke.
4. **Execution routing** — completed 2026-08-21: immutable bound-Conversation
   routing for File, Terminal, Job, Skill-script, and artifact reads through
   the Host Runner; fail-closed Runner identity/path revalidation; preserved
   approvals, cancellation, limits, and durable `host_workspace` presentation;
   focused cross-layer tests, targeted deployment, Host cwd/File/Job smoke,
   Runner-loss/recovery drill, and no Docker `/workspace` fallback.
5. **Permission enforcement and UI**: three durable presets, sandbox probes,
   Full access confirmation, active-turn locking, escalation and denial tests.
6. **Canary deployment and hardening**: focused cross-layer smoke, rollback
   drill, docs, automatic commits/builds, and live canary verification.

## Decision (ADR-lite)

**Context**: Supporting arbitrary Windows and WSL projects requires execution
outside Docker. Implementing native Windows and WSL execution engines at once
would duplicate packaging and sandbox work before the protocol is proven.

**Decision**: The first production release uses one ordinary-user WSL Host
Runner for both native WSL paths and Windows drives mounted under `/mnt/*`.
Windows folder selections are converted to their WSL execution path. The wire
protocol, durable workspace schema, and routing all carry `runnerId` and
platform/capability facts so a native Windows Runner can be added without a
schema or API replacement.

**Consequences**: The first release executes Windows-drive projects with the
WSL toolchain rather than native PowerShell/Windows tools. Permission probes
must specifically validate the target WSL kernel and DrvFS behavior and fail
closed when the promised sandbox mode is unavailable. Native Windows execution
remains a later compatible Runner implementation, not a first-release blocker.

The target WSL environment has Windows PowerShell interop and `wslpath`, but no
active user-systemd manager. Runner lifecycle therefore cannot depend on a
user `systemd` unit in the first release; the deployment wrapper owns startup,
stale PID/socket recovery, health verification, and the documented stop path.

### Workspace convergence decision

**Context**: neo-chat already exposes a Workspace concept whose records contain
prompt/files settings but no filesystem authority. Adding a second top-level
Project concept would leave two competing grouping systems; deleting or
recreating the existing records would lose user state.

**Decision**: Upgrade the existing visible Workspace in place. Legacy records
remain valid and are projected as `unbound`; binding adopts a Host Runner
directory while preserving the existing Workspace identity and settings. New
Workspaces are created through the directory picker and are born bound. The
primary UI exposes only this converged Workspace concept.

**Consequences**: Migration must bridge browser-persisted legacy Workspace
state into Backend durability idempotently. Agent execution is unavailable for
an unbound Workspace, but ordinary historical chat rendering and prompt/files
behavior remain available. Binding must never infer or guess a filesystem path
from a Workspace name.
