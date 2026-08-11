# MCP Client Design

## Data flow

```text
HTTP control API -> Service -> PostgreSQL/vault/connectors
Chat Tool loop   -> PrepareRun -> frozen snapshot -> ExecuteRound
ExecuteRound     -> Streamable HTTP or internal Runner -> normalized result
Result           -> PostgreSQL inline metadata + optional MinIO artifact
Cleanup worker   -> delete artifact -> delete/acknowledge durable row
```

## Components

- `manifest.go`: strict embedded catalog and administrator manifest authority.
- `safehttp.go`: public-HTTPS, DNS, redirect, and origin credential policy.
- `oauth.go`: PKCE, hashed single-use state, metadata/token flow, and refresh.
- `service.go`: grant, selection, snapshot, credential, lifecycle, and retention
  orchestration.
- `execution.go`: alias lookup, JSON-schema validation, read/write scheduling,
  retry policy, budgets, normalization, and timeline events.
- `connector.go` / `runner_connector.go`: remote MCP and internal stdio transport
  adapters.
- `repository_postgres.go`: durable authority and bounded projections.
- `handler.go`: authenticated camelCase product API with stable sanitized errors.

## Invariants

1. Current user and Workspace authority is checked before send and before each
   call; the run snapshot cannot expand after acceptance.
2. User-private endpoints remain public HTTPS across every DNS resolution and
   redirect. Credentials never cross an origin change.
3. Private-server annotations cannot grant read/idempotent scheduling authority.
4. Trusted reads may retry once only before a result is observed. Writes and
   unknown calls never retry; a dropped write becomes `outcome_unknown`.
5. Same-user write calls are serialized across runs. Trusted reads are bounded
   by per-user concurrency.
6. Arguments, results, tokens, and custom URLs are excluded from logs/metrics.
7. Artifact deletion precedes durable call deletion. Partial failure retains
   database authority for idempotent retry.
8. Retention and account-deletion cleanup continue when MCP execution is off.

## Limits

Defaults are 20 private servers/user, 8 selected servers/conversation, 32
exposed Tools/provider round, 32 calls/run, 8 rounds/run, 4 concurrent
calls/user, 30 seconds/call, 120 seconds/run, 256 KiB inline result, 25 MiB/item,
50 MiB/call, and 90-day audit retention.

## Deliberate non-goals

MCP Resources, Prompts, Sampling, Elicitation, legacy SSE transport, dynamic
client registration by default, an OpenAPI adapter, a public proxy, frontend
Tool execution, and per-call approval dialogs are outside this module.

## Design decisions and tradeoffs

- **Backend ownership over browser execution** prevents stale local state from
  becoming authorization, at the cost of a larger server persistence surface.
- **One frozen snapshot per run** prevents grant/schema drift during
  continuation, at the cost of refreshing changed Tool lists only between
  runs.
- **Object-before-row cleanup** preserves retry authority after MinIO failure,
  at the cost of temporarily retaining expired PostgreSQL rows.
- **No Plugin compatibility adapter** avoids a long-lived dual trust model;
  rollback keeps additive tables and prior images instead.

## Rollback

Disable MCP or a transport and retain migration `074`. Cleanup continues. Do
not revive the retired Plugin runtime or run destructive down migrations after
live Tool traffic.
