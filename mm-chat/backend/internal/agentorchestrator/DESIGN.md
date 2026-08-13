# Durable Agent Orchestrator Design
## Authority

PostgreSQL is the only Run, Step, Attempt, event sequence, lease and Kill
Switch authority. `agent_orchestrator_runtime` receives read plus exact
function execution and no table DML. `go_api_runtime` receives no G20.2
privilege because this group exposes no public API.

```text
canonical snapshot + ordered plan
  -> idempotent enqueue transaction
  -> immutable snapshot + current projection + append-only events
  -> generation lease
  -> heartbeat / transition under exact token digest + generation
  -> terminal retention
```

Redis is absent. A later ID-only wake hint may accelerate polling but cannot
authorize or recover a Run.

## Event and projection model

Every mutation locks the owning Run, consumes `next_sequence`, appends a
sanitized event and updates the projection in one transaction. The first legal
terminal event is final. A conflicting observation appends a `fact` event and
does not rewrite state.

Rebuild derives state, generation and next sequence from events. Lease tokens
are never event data: the caller sees the opaque token once while PostgreSQL
stores its SHA-256 digest. Restore/restart therefore treats current lease
projection as fencing input and never reconstructs credentials from logs.

## Kill Switch

Runs freeze a bounded list of applicable scope keys, including `global:*`,
`scheduler:*`, exact owner and exact Run. Each switch change is a new revision
and global epoch. Resolution takes all latest active matching records and
chooses `kill > cancel > deny_new`; an inactive narrow record cannot override
a broad deny.

G20.2 fences acquisition, heartbeat and progress. Physical process kill starts
only after the G20.3 Runner exists.

## Rollback and cleanup

Migration `084` down refuses while any Run/snapshot/event/Kill-Switch authority
exists. Application rollback leaves the inert additive schema applied. Bounded
retention deletes only terminal Runs before a caller-provided cutoff and
cascades their projection/event/snapshot set.
