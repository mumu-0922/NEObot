# DeepSeek Harness host, workspace, and permission architecture

Reference inspected: `deepseek-ai/deepseek-harness` at
`141eb6fef83422698aef7a981029e843e8161534` (2026-08-19).

## Permission presets

The shipped base composition declares three presentation presets in
`packages/bundle/base/cordis.patch.yml`:

* `read-only` -> sandbox `read-only`, approval `ask`
* `workspace-write` -> sandbox `workspace-write`, approval `ask`
* `danger-full-access` -> sandbox `danger-full-access`, approval `never`

`packages/interaction/permission-presets/src/index.ts` does not enforce these
itself. It records `permission/preset`, writes the independent
`sandbox/mode` and `approval/policy` session knobs, and projects their folded
state to clients. `packages/sandbox/sandbox-policy/src/session-mode.ts` makes
the sandbox mode a replayable per-session event.

`packages/client/ui-conversation/src/client/skeleton/PermissionSelect.tsx`
renders the current projection, locks the picker during an active turn, and
requires an explicit acknowledgement before selecting Full access.

Actual enforcement lives below the model Tool layer:

* filesystem calls use the sandboxed filesystem provider;
* shell calls use the sandboxed shell provider;
* Linux uses the local Landlock-backed sandbox path;
* Windows uses the restricted-token/ACL sandbox path;
* every enforcing capability receives the resolved session mode and workspace
  root for the call.

## Filesystem workspaces

`packages/workspace/workspace/` owns a durable registry. A workspace has a
stable UUID-like id, canonical `realpath`, display title, ordered session
account, and timestamps. The canonical path rather than the user spelling is
the uniqueness authority.

`packages/host/apiproxy/src/api-proxy.ts` resolves a selected workspace during
session creation and assigns `cwd = workspace.path`; the stored session cwd is
then validated when attaching the session to the workspace.

Directory interaction is a capability seam:

* `packages/host/directory-picker-native/` opens an OS-native picker on the
  machine running the Host process (Windows `IFileOpenDialog`, Linux
  Zenity/KDialog, macOS osascript).
* `packages/host/directory-picker-browse/` lists and creates directories in the
  filesystem visible to the Host process for remote/in-app browsing.
* `packages/client/ui-workspace/` owns adoption, grouping, rename/delete, and
  directory-flow UI rather than filesystem authority.

Therefore Harness does not bypass container filesystems. Its ordinary local
product topology runs the Host process directly in the user's OS environment;
if that Host were containerized, it would see only mounted paths too.

## Patterns to carry into neo-chat

* Separate preset presentation from independent sandbox and approval knobs.
* Persist effective permission and workspace identity per conversation/session.
* Make enforcement a Runner capability fact; hide or fail closed when missing.
* Canonicalize paths at the authority that owns the filesystem.
* Bind session cwd once and reject conflicting reuse.
* Keep directory-picker UI optional and capability-driven.
* Treat Full access as the Runner process user's authority, not an impossible
  promise of privileges the process does not own.
