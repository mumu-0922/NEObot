# Agent Host foundation

`agenthost` is the versioned control-plane foundation for the ordinary-user
WSL process that will execute Agent tools against real Host workspaces. The
first slice deliberately exposes only identity, capability discovery, and
canonical workspace resolution. It does **not** execute commands, browse
directories, open a native picker, or claim that any permission preset is
enforced.

## Responsibilities

- serve bounded HTTP/1.1 over a private Unix socket;
- authenticate every request with an independent bearer token;
- pin a stable `runnerId` on both sides of the connection;
- normalize Windows selections through the fixed `wslpath` executable;
- canonicalize WSL paths, resolve symlink aliases, and require a real
  directory;
- return an opaque directory fingerprint without exposing Host paths in
  errors;
- reject unsafe token files, socket directories, stale-socket races, unknown
  JSON fields, trailing JSON, and oversized messages.

## Quick usage

Build and run the Host process from the product root:

```bash
./scripts/agent-host.sh prepare
./scripts/agent-host.sh status
./scripts/agent-host.sh stop
```

The default runtime paths are gitignored and are created only when absent:

```text
.runtime/agent-host/agent-host
.runtime/agent-host/agent-host.sock
.runtime/agent-host/agent-host.pid
.runtime/agent-host/agent-host.log
secrets/agent-host-token
secrets/agent-host-runner-id
```

Use the Go client with the exact expected Runner identity:

```go
client, err := agenthost.NewClient(agenthost.ClientConfig{
    SocketPath:       "/run/mm-chat-agent-host/agent-host.sock",
    Token:            token,
    ExpectedRunnerID: "wsl-0123456789abcdef01234567",
})
if err != nil {
    return err
}
defer client.Close()

capabilities, err := client.Capabilities(ctx)
```

No Backend execution route uses this client yet. That activation is a later,
feature-gated slice and must never silently fall back to Docker `local_direct`
after a conversation becomes Host-bound.

## Files

```text
protocol.go    Versioned request, response, feature, and error shapes
handler.go     Authenticated bounded HTTP routes
client.go      Runner-id-pinned Unix-socket HTTP client
resolver.go    WSL/Windows path conversion and canonical identity
token.go       Strict token-file loading
unix.go        Private socket creation, stale cleanup, and safe close
validation.go  Shared protocol bounds and validation
```

See [DESIGN.md](./DESIGN.md), the
[protocol contract](../../../docs/contracts/agent-host-protocol.md), and the
[deployment runbook](../../../docs/deployment/agent-host-runner.md).
