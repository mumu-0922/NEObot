# Host Workspace domain design

## Goals

- Converge on the existing `workspaces.id` instead of creating a competing
  top-level Project concept.
- Preserve legacy browser settings and Conversation membership.
- Make Host directory identity Runner-authoritative and durable.
- Prevent a Conversation's effective Agent working directory from changing
  after execution begins.
- Open Agent-generated files from their current project location without
  creating duplicate export objects or trusting a browser-supplied Host path.
- Keep the current rollout fail closed until Docker Backend can authenticate to
  the Host Runner socket.

## Non-goals

- This module does not browse the Host filesystem or open a native picker.
- It delegates bound Agent Tools and the immutable effective permission to the
  pinned Host client; Host capability probes and the Runner enforce the mode.
- It never creates, renames, writes, or deletes a selected directory.
- It does not infer a filesystem path from a Workspace name.

## Architecture

```text
authenticated HTTP request
        |
        v
     Handler  -- strict JSON / generic errors
        |
        v
     Service  -- UUID/settings/revision/descriptor validation
       |  \
       |   `--> PathResolver --> Agent Host (canonicalization authority)
       v
PostgresRepository
       |
       +--> workspaces (settings + one-time Host binding)
       `--> conversations
             workspace_id       = mutable visible grouping
             agent_workspace_*  = immutable execution snapshot
```

The Backend owns user authorization and durability. The Host Runner owns path
conversion, `realpath`, directory probing, path classification, and the
Runner-bound directory fingerprint. The Service validates the returned
descriptor again before persistence.

Workspace-file reads start from an owner-scoped Workspace ID plus a normalized
relative path. The Service reloads the stored binding, calls the pinned Host
client with the persisted Runner/path/fingerprint tuple, and rejects a returned
path, version, or size that violates the request. HTTP responses are `no-store`
and `nosniff`, use a safe Content-Disposition and content-version ETag, and cap
one file at 50 MiB. Preview parsing is bounded independently: text and DOCX are
flattened to text, while XLSX exposes capped sheet/row/cell JSON. This keeps
archive decompression and renderer memory under server control.

For a bound Agent turn, Chat resolves this module's immutable execution
snapshot, verifies current Runner capability/identity, and adapts the existing
local Tool loop to `agenthost.Client`. Transport loss propagates as a Tool
failure; the adapter never invokes the Docker executor as a fallback.

## Persistence model

Migration `102_host_workspaces` extends `workspaces` in place. Legacy records
remain valid with all Host columns `NULL`; the API projects them as `unbound`.
A successful bind atomically fills the complete Host tuple and increments
`revision`. The partial unique index on owner, Runner, and fingerprint prevents
two active aliases of the same canonical directory.

`conversations.workspace_id` remains the current visible grouping. On the first
real Host Agent turn, `LockConversationExecutionWorkspace` copies the selected
Workspace's `runner_id`, `canonical_path`, and fingerprint into
`conversations.agent_workspace_*` under a row lock. Repeating the same lock is
idempotent; requesting another Workspace returns `ErrConversationLocked`.

This snapshot is not a second Project. It is the minimum authority needed to
prove that later UI grouping or Workspace metadata changes cannot silently
change an already-running Conversation's `cwd`.

Migration `103_chat_agent_permission_modes` adds the durable
`conversations.agent_permission_mode` authority with a default of
`workspace-write`. The dedicated permission mutation requires a bound Host
Workspace, a currently advertised Runner mode, no pending/streaming assistant
Turn, and explicit acknowledgement for Full access. Generic Conversation
metadata cannot update or shadow this field. Execution locking returns the
permission in the same immutable per-Turn binding as `cwd`.

## Design decisions and trade-offs

- **In-place convergence over a second Project table** preserves visible
  identity and settings, at the cost of an additive migration on an older MCP
  registry.
- **Immutable execution snapshot over live grouping lookup** prevents `cwd`
  drift, at the cost of retaining a small amount of duplicated authority on
  each executed Conversation.
- **Fail-closed Host routing over Docker path fallback** preserves execution
  authority across Runner loss and identity drift.
- **Current workspace file over copied chat artifact** makes Open the primary
  action and keeps Download secondary. Historical messages retain the emitted
  SHA-256 version; the viewer explicitly warns when current bytes have changed.

## Concurrency and authorization

- Every repository query is scoped to `auth.UserOrDevelopment(ctx).ID`.
- File reads resolve the Workspace under that owner before invoking Host I/O;
  caller paths cannot select a Runner, mount, canonical Host directory, or
  object-store key.
- Bind authorizes the owner, revision, and unbound state before invoking the
  Host resolver, so rejected records cannot be used as filesystem probes.
- Settings, bind, and delete mutations require an exact positive revision.
- Binding is one-way; a bound Workspace cannot be rebound in place.
- Execution locking uses `SELECT ... FOR UPDATE` on the Conversation and
  `FOR SHARE` on the bound Workspace in one transaction.
- Permission changes are owner-scoped and rejected while an assistant Message
  is `pending` or `streaming`; a Turn that already captured its execution
  binding cannot be changed through browser metadata.
- Runtime grants are column-scoped to fields used by this repository. The
  runtime role cannot change Workspace ownership or delete database rows.

## Validation and errors

The Handler rejects unknown/duplicate JSON keys, trailing documents, query
strings, wrong media types, and oversized bodies. Settings and Host descriptors
have explicit byte/count/shape bounds. Errors returned to browsers are stable
and generic; submitted Host paths and database or OS errors are never echoed.

Important states:

- `ErrDisabled`: PostgreSQL or the Host resolver is unavailable.
- `ErrRevisionConflict`: the client wrote against stale state.
- `ErrAlreadyBound`: one-time binding has already completed.
- `ErrDirectoryAlreadyRegistered`: the canonical directory alias already has
  an active Workspace for the owner and Runner.
- `ErrWorkspaceUnbound`: execution locking was requested before binding.
- `ErrConversationLocked`: visible grouping conflicts with the immutable
  execution snapshot.
- `ErrPermissionAcknowledgement`: Full access lacked explicit acknowledgement.
- `ErrPermissionUnavailable`: the live pinned Host does not advertise the mode.
- `ErrPermissionLocked`: an assistant Turn is active.
- `ErrFileNotFound`, `ErrFileTooLarge`, and `ErrInvalidPath`: a bounded file
  request failed without disclosing Host paths or another owner's existence.

## Rollout and rollback

Compose injects one pinned `agenthost.Client` through this Service for both
path resolution and Tool execution. The only Host mount in Backend remains the
private Runner socket and identity secrets; project directories stay outside
Docker.

Migration `102.down` succeeds only when no imported settings, Host binding, or
Conversation execution snapshot exists. Otherwise it raises
`HOST_WORKSPACE_ROLLBACK_BLOCKED`; rollback must preserve the database and use
the compatible application image. Soft deletion never touches Host files.
Migration `103.down` first refuses to erase any non-default permission choice;
after a clean `103.down`, the existing migration-102 rollback guard still
applies.

## Known limitations

- Foreground Host Terminal chunks arrive at Backend after completion in this
  slice; final durable cards are preserved, but live per-chunk transport is not.
- The first Runner target executes both WSL and mounted Windows projects with
  the WSL toolchain; a native Windows execution engine is not yet present.

## Verification

- Unit tests cover settings validation, strict HTTP parsing, generic errors,
  Runner descriptor validation, and route behavior.
- PostgreSQL 17 integration runs as `go_api_runtime`, proves import replay,
  directory alias uniqueness, cross-user denial, immutable execution locking,
  in-use delete denial, empty down/re-up, and guarded rollback with data.
- Focused race tests and `go vet` cover this module and its wiring packages.
- File tests cover owner isolation, traversal rejection before Host I/O,
  strict query parsing, response headers, XLSX bounds, and oversized archives.

## Change history

- **2026-08-21**: introduced migration-102 in-place Workspace convergence,
  strict API, one-time Host binding contract, and Conversation execution lock.
- **2026-08-21**: routed immutable bound Workspace Tools through the pinned Host
  while preserving ungrouped legacy `local_direct` and fail-closed loss.
- **2026-08-21**: added migration-103 durable permissions, active-Turn locking,
  capability admission, and immutable permission propagation to the Host.
- **2026-08-25**: added pinned owner-scoped workspace-file content/preview
  reads, bounded DOCX/XLSX parsing, version drift signaling, and no-copy open.
