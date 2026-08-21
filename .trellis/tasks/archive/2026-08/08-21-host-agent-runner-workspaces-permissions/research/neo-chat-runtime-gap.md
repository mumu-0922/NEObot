# neo-chat runtime gap for Harness-compatible host workspaces

## Current state

* `mm-chat/frontend/src/lib/chat/types.ts` defines `Workspace` as a client-side
  name/system-prompt/files grouping. It has no canonical filesystem path or
  Backend execution authority.
* `mm-chat/frontend/src/store/core/chatStore.ts` creates, updates, deletes, and
  reassigns these Workspace records in frontend state.
* `mm-chat/backend/internal/localskills/runtime.go` owns one process-wide
  `WorkspaceRoot`; file tools resolve under it and terminal starts there.
* The local runtime explicitly documents itself as guardrails rather than
  isolation. Terminal launches a normal shell in the Backend container.
* `mm-chat/compose.single-server.yml` supplies one static bind at `/workspace`.
  A running Docker container cannot adopt a new arbitrary Windows/WSL host
  directory merely because the browser submitted its path.
* The live deployment currently binds one host project to `/workspace`.
* Durable Chat Agent approvals, approval wait/resume, Tool events, cancellation,
  process output, and transcript replay already exist and should remain the
  control plane rather than being duplicated inside the Runner.

## Required seams

1. A Host Runner identity and authenticated local transport.
2. Runner capability/health registration with fail-closed routing.
3. Runner-owned path selection, canonicalization, and execution.
4. Backend-owned durable workspace records and conversation binding.
5. A versioned execution protocol with idempotency, deadlines, cancellation,
   bounded output, and stable error codes.
6. Runner sandbox modes with startup/runtime enforcement probes.
7. Frontend filesystem Workspace and access-mode projections.
8. Migration compatibility for legacy conversations and the existing
   prompt/files Workspace concept.

## Feasible architecture approaches

### A. WSL Host Runner with versioned multi-runner protocol (recommended first release)

Run one ordinary-user Go daemon in WSL. It accesses native WSL projects and
Windows drives under `/mnt/*`, invokes a Windows native folder-picker helper
when needed, and normalizes selected Windows paths through `wslpath`. Backend
communicates over a permission-restricted Unix socket plus request-level
authentication. The protocol includes `runnerId` so a native Windows Runner
can be added later without schema replacement.

Pros: smallest deployable Host plane, matches the current WSL/Docker topology,
reuses the Go toolchain, and avoids exposing a network listener.

Cons: Windows projects execute with Linux/WSL tools initially; Landlock/DrvFS
enforcement must be probed and fail closed rather than assumed.

### B. WSL Runner plus native Windows Runner in the first release

Register two Host daemons and bind each workspace to one. Windows projects use
native PowerShell/ACL semantics, while WSL projects use Linux/Landlock.

Pros: closest cross-platform execution fidelity.

Cons: doubles packaging, lifecycle, transport, sandbox, path, and integration
surface before the workspace/control-plane contract has stabilized.

### C. Run the whole Backend directly on the host

Remove the execution boundary by moving the API Backend out of Docker.

Pros: direct filesystem access.

Cons: couples Web/API lifecycle and broad filesystem authority, complicates the
existing Compose deployment, weakens least privilege, and creates a much larger
rollback surface. Not recommended.
