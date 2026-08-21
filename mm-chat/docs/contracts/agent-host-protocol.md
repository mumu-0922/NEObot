# Agent Host protocol foundation

## Status

Protocol version `1` currently defines only the Host identity/capability and
workspace-resolution control plane. It is not an execution protocol and does
not change the existing Docker `local_direct` Agent runtime.

## Transport and authentication

- HTTP/1.1 over an absolute Unix socket path shorter than 101 bytes.
- Socket parent: canonical, owner-matched directory with exact mode `0700`.
- Socket: owner-matched Unix socket with exact mode `0600`.
- Every request: `Authorization: Bearer <agent-host-token>`.
- The token is independent from MCP and is loaded from an absolute,
  owner-matched, regular, non-symlink file with exact mode `0600`.
- Request bodies are at most 16 KiB; responses are at most 64 KiB.
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
    "directoryBrowse": false,
    "nativeDirectoryPicker": false,
    "windowsPathInterop": true,
    "execution": false,
    "permissionModes": []
  },
  "limits": {
    "maxRequestBytes": 16384,
    "maxResponseBytes": 65536,
    "maxPathBytes": 4096
  }
}
```

The Host must not list `read-only`, `workspace-write`, or
`danger-full-access` until the corresponding enforcement probes pass on the
target WSL kernel and filesystem.

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
- `WORKSPACE_PATH_INVALID`
- `WORKSPACE_PATH_UNAVAILABLE`

Messages never include the submitted Host path, token, OS error, or command
output. The Backend must treat unknown shapes, identity mismatches, and limit
violations as protocol failures.

## Forward contract

Future browse, picker, and bounded NDJSON execution routes will remain under a
versioned internal namespace and carry `runnerId`, workspace identity, and
permission mode. Runner loss must fail closed; an operation with unknown
outcome is never automatically replayed. Host-bound conversations never fall
back to Docker execution.
