# Host Workspace domain design

## Goals

- Converge on the existing `workspaces.id` instead of creating a competing
  top-level Project concept.
- Preserve legacy browser settings and Conversation membership.
- Make Host directory identity Runner-authoritative and durable.
- Prevent a Conversation's effective Agent working directory from changing
  after execution begins.
- Keep the current rollout fail closed until Docker Backend can authenticate to
  the Host Runner socket.

## Non-goals

- This module does not browse the Host filesystem or open a native picker.
- It does not execute Agent Tools or enforce permission presets.
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

## Design decisions and trade-offs

- **In-place convergence over a second Project table** preserves visible
  identity and settings, at the cost of an additive migration on an older MCP
  registry.
- **Immutable execution snapshot over live grouping lookup** prevents `cwd`
  drift, at the cost of retaining a small amount of duplicated authority on
  each executed Conversation.
- **Dark fail-closed resolver over Docker path fallback** delays binding UI by
  one slice, but prevents the product from claiming arbitrary Host access
  before the socket boundary exists.

## Concurrency and authorization

- Every repository query is scoped to `auth.UserOrDevelopment(ctx).ID`.
- Bind authorizes the owner, revision, and unbound state before invoking the
  Host resolver, so rejected records cannot be used as filesystem probes.
- Settings, bind, and delete mutations require an exact positive revision.
- Binding is one-way; a bound Workspace cannot be rebound in place.
- Execution locking uses `SELECT ... FOR UPDATE` on the Conversation and
  `FOR SHARE` on the bound Workspace in one transaction.
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

## Rollout and rollback

This slice registers the HTTP API but injects no `PathResolver`, so binding is
unavailable by construction. A later Compose slice must mount only the exact
private Runner socket/token and create an authenticated `agenthost.Client`.

Migration `102.down` succeeds only when no imported settings, Host binding, or
Conversation execution snapshot exists. Otherwise it raises
`HOST_WORKSPACE_ROLLBACK_BLOCKED`; rollback must preserve the database and use
the compatible application image. Soft deletion never touches Host files.

## Known limitations

- The current Backend has no Host socket/token mount, so binding is unavailable.
- Native picker, directory browsing, Tool routing, and three-mode permission
  enforcement belong to later committed slices.
- The first Runner target executes both WSL and mounted Windows projects with
  the WSL toolchain; a native Windows execution engine is not yet present.

## Verification

- Unit tests cover settings validation, strict HTTP parsing, generic errors,
  Runner descriptor validation, and route behavior.
- PostgreSQL 17 integration runs as `go_api_runtime`, proves import replay,
  directory alias uniqueness, cross-user denial, immutable execution locking,
  in-use delete denial, empty down/re-up, and guarded rollback with data.
- Focused race tests and `go vet` cover this module and its wiring packages.

## Change history

- **2026-08-21**: introduced migration-102 in-place Workspace convergence,
  strict API, one-time Host binding contract, and Conversation execution lock.
