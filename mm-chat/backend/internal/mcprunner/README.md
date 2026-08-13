# MCP Stdio Runner

`mcprunner` hosts administrator-approved stdio MCP servers behind a private,
independently authenticated HTTP control API. One Runner container starts child
servers on demand; it does not run one container or permanent process per
server.

## Responsibilities

- Accept only manifest-approved server IDs.
- Start at most four fixed argv-based child processes; dynamic npm may use only
  the Runner-owned `npx` launcher, never a caller-selected shell command.
- Create a private work directory and minimal environment per server.
- Reuse a healthy session, reap it after its idle timeout, and enforce maximum
  lifetime.
- Kill the entire process group on cancellation, crash, expiry, or shutdown.
- Expose only health, Tool listing, and Tool call operations to the backend.

The package does not own users, grants, credentials, PostgreSQL, MinIO, or
provider continuation. Those remain in `mcpclient` and the chat Tool loop.

## Usage

`cmd/mcp-runner` loads the validated manifest and constructs the manager and
handler:

```go
manager, err := mcprunner.NewManager(config, manifestServers)
handler, err := mcprunner.NewHandler(manager, independentRunnerToken)
```

Do not embed the handler in the public API listener; it belongs only on the
private Runner service.

## Verification

```bash
cd mm-chat/backend
go test ./cmd/mcp-runner ./internal/mcprunner
```

See [`DESIGN.md`](./DESIGN.md) and
[`../../../docs/deployment/mcp-runner.md`](../../../docs/deployment/mcp-runner.md).
