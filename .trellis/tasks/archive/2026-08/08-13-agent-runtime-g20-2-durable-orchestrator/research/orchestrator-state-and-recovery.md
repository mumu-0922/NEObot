# Orchestrator State, Recovery, and Kill-Switch Boundary
## Evidence inspected

- `mm-chat/docs/contracts/agent-runtime.md`
- `mm-chat/docs/architecture/agent-runtime.md`
- `mm-chat/docs/deployment/agent-runtime.md`
- `mm-chat/docs/contracts/schemas/neo-run-event.schema.json`
- `mm-chat/backend/internal/mcpclient/` snapshot patterns
- `mm-chat/backend/migrations/074_mcp_tools_foundation.up.sql`

## Decisions

### State/event model

- Store immutable canonical Run snapshots separately from mutable projections.
- Store Run, ordered Step and generation-bound Attempt projections plus a
  per-Run monotonically sequenced append-only event ledger.
- The first legal terminal transition is final. A conflicting observation may
  append a sanitized `fact` event but cannot rewrite that state.
- Projection rebuild recomputes Run/Step/Attempt status, Step generation and
  next sequence from the event ledger. Opaque live lease tokens are not event
  data and are never reconstructed from diagnostics.

### Recovery

- Restart discovery reads nonterminal Runs and current live/expired Attempts
  directly from PostgreSQL. No in-process or Redis state is needed.
- An expired Attempt is reclaimed only through the atomic lease function; a new
  Attempt has a higher generation and a distinct opaque token.
- Retention removes only terminal Runs older than a cutoff through one bounded
  owner function. Cascades remove the snapshot/projections/events together.

### Kill Switch

- Persist revisioned immutable deny records for the contract hierarchy.
- Disable/removal is a new revision, never deletion of incident history.
- Resolution considers every applicable scope and returns the strongest active
  mode (`kill > cancel > deny_new`); no narrower record can cancel a broader
  deny.
- `deny_new` blocks a new lease. `cancel`/`kill` additionally reject heartbeat
  and non-terminal Attempt progress. G20.2 provides durable fencing only; no
  Runner or Sandbox exists to receive a physical kill.

### Exposure boundary

G20.2 exposes a typed internal Go service/repository seam and no HTTP route,
feature flag, worker binary or startup wiring. This prevents a queued fixture
from being presented as executable Runtime before G20.3/G20.4.
