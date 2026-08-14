# G21.2 Runner-to-Broker relay boundary research

## Existing boundary

- `internal/agentrunner.Service` already validates the original caller mTLS
  identity, signed authority ticket, exact request fingerprint, Attempt lease,
  snapshot and Kill-Switch epoch before calling its injected `BrokerRelay`.
- `neo-runnerd` still injects the fail-closed `unavailableBrokerRelay`; no
  production process currently receives a relayed Prepare or Commit.
- The relay interface currently receives only the typed Prepare/Commit body.
  The body retains the complete signed authority ticket, including the
  original caller, Runner, request, nonce, Attempt, lease-token digest,
  snapshot and epoch bindings.
- The host Runner intentionally has no PostgreSQL, object-store, MCP, Provider
  or vault credential. Putting the Broker service in `neo-runnerd` would break
  this boundary even if the Sandbox remained isolated.

## Considered approaches

### In-process Broker inside `neo-runnerd`

Rejected. It is mechanically small but gives the host daemon database and
executor credentials and makes a Runner compromise an authority compromise.

### Broker calls that bypass Runner

Rejected. A canary worker could call `agentbroker.Service` directly, but that
would not prove the existing Runner Prepare/Commit seam or caller-specific
ingress policy.

### Private mTLS relay service (recommended)

Use a dedicated private HTTPS endpoint owned by the G21.2 canary process.
`neo-runnerd` forwards only strict Prepare/Commit bodies after its normal
authority check. The relay endpoint:

- accepts only a separate Runner relay mTLS identity;
- permits only Prepare and Commit on one fixed path;
- independently verifies the original Ed25519 authority ticket and exact
  unsigned body fingerprint;
- resolves the user, Grant and Registry only from a strict mounted synthetic
  plan, never from caller-supplied authority JSON;
- maps the request to the durable `agentbroker` repository;
- returns only the existing bounded Prepare/Commit result shapes.

`neo-runnerd` receives only relay URL/trust material. It never receives the
Broker database URL, object-store keys or MCP Runner token. The Sandbox receives
neither side's mTLS keys and keeps `networkMode=none`.

## Identity and method split

- Keep `spiffe://neo-chat/agent-runtime-control` at
  `probe/list/reconcile`.
- Keep `spiffe://neo-chat/agent-runtime-root-canary` at
  `probe/list/reconcile/launch/heartbeat/cancel`.
- Add `spiffe://neo-chat/agent-runtime-broker-canary` for the separately
  activated G21.2 flow. It may use the lifecycle methods plus
  `prepare/commit` only.
- Add a different outbound Runner relay identity for Runner-to-Broker mTLS.
  It is transport identity only and owns no database role.

The G21.2 database LOGIN recursively inherits exactly
`agent_orchestrator_runtime`, `agent_runner_control`, `agent_effect_control`
and `agent_artifact_control`. The relay transport identity is not that LOGIN.

## Failure contract

- Network failure before a relay response maps to `RUNTIME_UNAVAILABLE`; it
  never grants authority locally.
- A possible executor send with no exact status becomes durable
  `outcome_unknown`; neither Runner relay nor canary loop retries Commit under
  another key.
- A relay restart recovers intent state from PostgreSQL. Exact Runner RPC
  replay remains content-free and returns the locally fenced response.
- Any activation, identity, method, ticket, plan or endpoint drift fails
  before Broker execution.
