# MCP Client

`mcpclient` is Neo Chat's server-authoritative MCP Tools subsystem. It owns
catalog/manifest loading, private server validation, grants and selections,
credential/OAuth lifecycle, remote and Runner transports, immutable run
snapshots, scheduling, normalized results, persistence, retention, and the
public `/v1/mcp/*` handler.

## Boundaries

- Neo Chat is an MCP client, never a public MCP server or proxy.
- The browser displays state but does not authorize or execute Tools.
- Remote private servers require public HTTPS and the hardened SSRF client.
- Stdio execution is delegated only to the internal `mcprunner` service.
- Credentials remain encrypted server-side and are never serialized.
- Tool output is untrusted data and cannot change the frozen run snapshot.

## Main entry points

| Entry point | Responsibility |
| --- | --- |
| `NewService` | Compose repositories, connectors, vault, objects, limits, and clocks. |
| `NewHandler` | Serve strict, no-store `/v1/mcp/*` product routes. |
| `LoadCatalog` / `LoadManifest` | Load release and administrator definitions with strict validation. |
| `PrepareRun` | Resolve grants/selection and persist an immutable execution snapshot. |
| `ExecuteRound` | Validate, schedule, execute, normalize, persist, and emit Tool calls. |
| `PruneExpiredData` | Delete object artifacts before expired durable rows. |
| `RunRetention` | Run cleanup independently from the MCP execution kill switch. |

`PostgresRepository` stores authority introduced by migration `074`. Large or
media result bytes use the `mcp-results/` object namespace; only validated
coordinates may be deleted.

## Usage

Runtime wiring belongs in `cmd/api`; callers inject existing repository,
vault, object-store, and connector implementations rather than constructing
parallel infrastructure:

```go
service, err := mcpclient.NewService(
    config, repository, connector, vault, objects, catalog, manifest,
)
handler := mcpclient.NewHandler(service)
```

See `cmd/api/main.go` for dependency construction and disabled-subsystem
behavior.

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/mcpclient ./internal/migration
cd ..
bash scripts/verify-mcp-postgres17.sh
```

See [`DESIGN.md`](./DESIGN.md),
[`../../../docs/architecture/mcp-tools.md`](../../../docs/architecture/mcp-tools.md),
and [`../../../docs/contracts/mcp-tools-api.md`](../../../docs/contracts/mcp-tools-api.md).
