# Agent Host protocol foundation

## Status

Protocol version `1` defines Host identity/capabilities, canonical Workspace
resolution, WSL directory browsing, the native Windows directory picker, and
bounded Agent Tool execution in an immutable Host Workspace. Permission modes
are capability facts produced only after live enforcement probes pass.

## Transport and authentication

- HTTP/1.1 over an absolute Unix socket path shorter than 101 bytes.
- Socket parent: canonical, owner-matched directory with exact mode `0700`.
- Socket: owner-matched Unix socket with exact mode `0600`.
- Every request: `Authorization: Bearer <agent-host-token>`.
- The token is independent from MCP and is loaded from an absolute,
  owner-matched, regular, non-symlink file with exact mode `0600`.
- Control request bodies are at most 16 KiB and responses are at most 64 KiB.
  Tool execution has independent 128 KiB request and 72 MiB response bounds;
  the larger response exists only for the already-bounded artifact body.
- JSON is strict: unknown fields and multiple/trailing documents are rejected.
- Successful responses carry the expected stable `runnerId`.

## Routes

### `GET /internal/v1/capabilities`

Returns:

```json
{
  "protocolVersion": 1,
  "runnerId": "wsl-0123456789abcdef01234567",
  "version": "0123456789ab",
  "platform": "linux-wsl",
  "architecture": "amd64",
  "features": {
    "workspaceResolve": true,
    "directoryBrowse": true,
    "nativeDirectoryPicker": true,
    "windowsPathInterop": true,
    "execution": true,
    "permissionModes": ["read-only", "workspace-write", "danger-full-access"]
  },
  "limits": {
    "maxRequestBytes": 16384,
    "maxResponseBytes": 65536,
    "maxPathBytes": 4096
  }
}
```

The Host must not list `read-only`, `workspace-write`, or
`danger-full-access` until the exact Bubblewrap execution boundary passes on
WSL storage and the live DrvFS probe target. `execution=true` means the Tool
route is active; each request must still carry one advertised permission mode.

### `POST /internal/v1/workspaces/resolve`

Request:

```json
{"protocolVersion":1,"path":"D:\\projects\\neo-chat"}
```

Response:

```json
{
  "protocolVersion": 1,
  "runnerId": "wsl-0123456789abcdef01234567",
  "workspace": {
    "canonicalPath": "/mnt/d/projects/neo-chat",
    "displayPath": "D:\\projects\\neo-chat",
    "pathKind": "windows-mounted",
    "directoryFingerprint": "sha256:<64-lowercase-hex>"
  }
}
```

Linux/WSL inputs use their canonical path as `displayPath` and `pathKind=wsl`.
Windows inputs use `wslpath -u -- <path>` without a shell and retain the
original display string. Canonicalization resolves symlinks and requires an
existing directory. The fingerprint binds the Runner id and canonical string;
it is a durable deduplication key, not a filesystem integrity proof.

### `POST /internal/v1/directories/browse`

The request contains `protocolVersion` and an optional absolute `path`. An
empty path starts at the ordinary Host user's home directory. The Host resolves
the current directory and returns its canonical/display path, optional parent,
and at most 256 sorted directory-only entries. Files are never returned.

```json
{"protocolVersion":1,"path":"/home/user"}
```

```json
{
  "protocolVersion": 1,
  "runnerId": "wsl-0123456789abcdef01234567",
  "path": "/home/user",
  "displayPath": "/home/user",
  "pathKind": "wsl",
  "parentPath": "/home",
  "entries": [
    {"name":"project","path":"/home/user/project","displayPath":"/home/user/project","pathKind":"wsl"}
  ]
}
```

### `POST /internal/v1/directories/pick-native`

The Host invokes a fixed PowerShell/WinForms folder picker and never
interpolates caller text into the command. A selection is converted through
the same `wslpath` and canonical Workspace resolver. Cancellation is a
successful `{"cancelled":true}` response. The Backend uses a dedicated bounded
270-second HTTP client for this human interaction, below the frontend proxy's
five-minute ceiling, while ordinary control calls retain the short timeout.

### `POST /internal/v1/tools/execute`

The Docker Backend sends one already-authorized Tool call with the immutable
Conversation Workspace snapshot:

```json
{
  "protocolVersion": 1,
  "workspace": {
    "canonicalPath": "/mnt/d/projects/neo-chat",
    "directoryFingerprint": "sha256:<64-lowercase-hex>"
  },
  "scope": {"userId":"<uuid>","conversationId":"<uuid>"},
  "tool": "terminal",
  "arguments": {"command":"pwd","workingDir":"","timeoutSeconds":30},
  "permissionMode": "workspace-write",
  "approved": false,
  "activeSkillRoot": "<relative-materialized-skill-root>"
}
```

Supported operations are `terminal`, `file_read`, `file_write`, `file_edit`,
`file_search`, `artifact_read`, `job_start`, `job_list`, `job_output`,
`job_kill`, and `job_notices`. Before every call the Host canonicalizes the
persisted path again and requires an exact canonical-path/fingerprint match.
It creates one bounded `localskills.Executor` per exact Workspace authority,
rooted at the Host directory, and preserves CAS/symlink defenses, smart
approval, hard blocks, process-group cancellation, time/output limits, and
process-local Jobs.

Permission enforcement is below model Tool arguments:

- `read-only`: Terminal runs with the Host root read-only; `file_write` and
  `file_edit` are rejected before filesystem mutation.
- `workspace-write`: Terminal sees the Host root read-only and receives one
  exact read-write bind for the canonical Workspace. File Tools remain rooted
  to that Workspace with existing CAS and symlink defenses.
- `danger-full-access`: Terminal uses the ordinary Host process authority and
  smart approval is disabled. It never invokes `sudo` or elevates privileges.

The Bubblewrap modes are write boundaries, not confidentiality or network
isolation: commands may read resources already readable by the Host user.

The response carries the pinned Runner identity and one strict typed result:

```json
{"protocolVersion":1,"runnerId":"wsl-0123456789abcdef01234567","result":{"exitCode":0,"stdout":"$NEO_CHAT_WORKSPACE\n","stderr":"","timedOut":false,"truncated":false,"durationMillis":4}}
```

Host paths and materialized Skill paths are redacted before results return.
This slice uses one bounded request/response exchange. Foreground Terminal
output is replayed into the existing callback after completion; durable final
transcript cards remain authoritative, but per-chunk Host NDJSON is not yet
implemented.

## Errors

Errors use:

```json
{"error":{"code":"WORKSPACE_PATH_UNAVAILABLE","message":"Workspace path is unavailable"}}
```

Stable codes in v1 are:

- `AGENT_HOST_UNAUTHORIZED`
- `AGENT_HOST_REQUEST_INVALID`
- `AGENT_HOST_PROTOCOL_UNSUPPORTED`
- `AGENT_HOST_METHOD_NOT_ALLOWED`
- `AGENT_HOST_ROUTE_NOT_FOUND`
- `WORKSPACE_RESOLVE_UNAVAILABLE`
- `WINDOWS_PATH_INTEROP_UNAVAILABLE`
- `DIRECTORY_BROWSE_UNAVAILABLE`
- `NATIVE_DIRECTORY_PICKER_UNAVAILABLE`
- `HOST_EXECUTION_UNAVAILABLE`
- `WORKSPACE_AUTHORITY_INVALID`
- `TOOL_NOT_AVAILABLE`
- `EXECUTION_FAILED`
- `EXECUTION_RESULT_INVALID`
- `APPROVAL_REQUIRED`
- `COMMAND_BLOCKED`
- `PERMISSION_DENIED`
- `ARGUMENTS_INVALID`
- `RUNTIME_BUSY`
- `JOB_NOT_FOUND`
- `JOB_SCOPE_INVALID`
- `FILE_NOT_FOUND`
- `FILE_TOO_LARGE`
- `INVALID_UTF8`
- `VERSION_CONFLICT`
- `EDIT_CONFLICT`
- `PATH_INVALID`
- `WORKSPACE_PATH_INVALID`
- `WORKSPACE_PATH_UNAVAILABLE`

Messages never include the submitted Host path, token, OS error, or command
output. The Backend must treat unknown shapes, identity mismatches, and limit
violations as protocol failures.

## Forward contract

Bounded NDJSON may replace the buffered foreground response without changing durable
Tool result authority. Runner loss fails closed; an operation with unknown
outcome is never automatically replayed. Host-bound conversations never fall
back to Docker execution.
