# Host Workspace domain

`hostworkspace` upgrades neo-chat's existing visible Workspace identity into a
durable Backend record that can later adopt exactly one Host Runner directory.
It preserves the browser-era name, system prompt, files, color, Search, and
Reasoning settings while adding revisioned Host binding and Conversation
execution authority.

## Responsibilities

- Import legacy browser Workspaces idempotently as `unbound` records.
- List, read, and compare-and-set update current-user Workspace settings.
- Ask a Host-owned resolver to canonicalize a selected directory, then bind the
  returned Runner/path/fingerprint tuple once.
- Keep visible Conversation grouping separate from the immutable execution
  snapshot captured when Agent execution first starts.
- Read one bound workspace file through the pinned Host authority for secure
  inline preview or explicit download without copying it into object storage.
- Soft-delete registrations without deleting or mutating project directories.
- Reject cross-user access, stale revisions, duplicate canonical directories,
  and execution-binding drift.

## Runtime status

The repository and `/v1/workspaces*` API are active after migration `102`.
Backend-to-Host socket wiring is intentionally not part of this slice, so the
production service is constructed without a `PathResolver`: settings and
legacy import work, while `POST /v1/workspaces/{id}/bind` fails closed with
`503 HOST_WORKSPACE_UNAVAILABLE`.

## Main types

- `Service`: validation and domain orchestration.
- `PostgresRepository`: current-user persistence and transactional execution
  locking.
- `Handler`: strict bounded JSON and owner-scoped file content/preview API.
- `PathResolver`: the narrow Host Runner canonicalization dependency.

## Usage example

```go
repository := hostworkspace.NewPostgresRepository(db)
service := hostworkspace.NewService(repository, nil) // dark: bind returns 503
handler := hostworkspace.NewHandler(service)
```

The later socket rollout replaces `nil` with a pinned `agenthost.Client`; it
does not replace the Workspace repository or public API.

Bound deployments expose
`GET /v1/workspaces/{id}/files/content?path=...` and
`GET /v1/workspaces/{id}/files/preview?path=...`. Both accept only a clean
workspace-relative path and revalidate the persisted Runner and mount
fingerprint before reading. Preview supports bounded text, DOCX, and XLSX;
binary inline media uses the content route and download is opt-in.

See [DESIGN.md](./DESIGN.md) and
[`docs/contracts/host-workspaces-api.md`](../../../docs/contracts/host-workspaces-api.md).
