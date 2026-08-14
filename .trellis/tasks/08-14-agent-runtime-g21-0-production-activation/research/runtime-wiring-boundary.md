# G21.0 runtime wiring boundary

## Current executable seams

- `cmd/neo-runnerd` already owns the credential-free host daemon and strict
  mTLS `neo.runner-rpc/v1` endpoint.
- `internal/agentrunner.RPCClient` supports TLS 1.3, private/loopback targets,
  client certificates and pinned server trust, but no production process
  constructs it.
- `PostgresControlRepository` is intentionally executable only by the
  `agent_runner_control` role. `go_api_runtime` cannot assume or inherit that
  role, so wiring the Runner into `cmd/api` would violate the existing least-
  privilege contract.
- `RecoverySandboxes` plus Runner `probe`, `list` and `reconcile` are sufficient
  for a control-only maintenance loop. Root Run acquisition, authority-ticket
  issuance and `launch` remain a later slice.

## Recommended boundary

Add a dedicated `mm-chat-agent-runtime-control` process in its own Compose
profile. It receives only its dedicated PostgreSQL URL, Runner mTLS files,
release/activation evidence and non-secret policy. It never receives Provider,
vault, object-store, MCP or application API credentials.

The worker revalidates activation evidence before every cycle, probes the
exact Runner, obtains the database recovery inventory, reconciles the remote
inventory and checks the post-reconcile result. Any drift terminates the worker
and makes health fail closed. It does not acquire Steps, issue launch authority,
call Broker effects or expose an HTTP route.

## Persistence boundary

G21.0 adds no migration. It uses migration `085`'s existing recovery function
under `agent_runner_control`. Runner request/Sandbox retention stays an operator
function until a later migration explicitly creates a bounded maintenance
role. Avoid granting `agent_runner_prune` broadly merely to make this baseline
look complete.
