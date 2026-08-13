# Existing Seams and G20.4 Fit

## Reusable code

- `internal/agentorchestrator`: Run/Step/Attempt generation, lease-token digest,
  immutable events and Kill Switch resolution.
- `internal/agentrunner`: strict `neo.runner-rpc/v1`, authority tickets, local
  fsync replay, exact Sandbox lifecycle and per-Attempt Artifact intake.
- `internal/mcpclient`: strict Tool schemas, private/remote connectors,
  same-user write serialization, SSRF-safe HTTP and bounded result storage.
- `internal/providersecrets`: encrypted Provider credential storage pattern.
- `internal/jobartifacts` and `internal/storage`: object-before-row cleanup and
  bounded object-store interfaces.
- `internal/strictjson`: duplicate/unknown/trailing JSON denial.

## Boundaries that must remain separate

Current Chat MCP execution records are scoped to a Conversation/Message and may
dispatch a write directly. They are not Agent Runtime Prepare/Commit authority
and must not be retrofitted into an effect ledger. The Agent Broker may adapt a
registered MCP executor only after durable Commit authorization and must never
retry an ambiguous write.

`neo-runnerd` has no PostgreSQL, object-store or vault credentials. Adding
Prepare/Commit shapes to Runner RPC cannot make the daemon the durable intent
owner. The daemon may validate the exact authority ticket and relay a bounded
request to an injected control-plane Broker interface; production wiring stays
held until an authenticated private channel and the exact-host suite exist.

## Tool Registry

Registry construction should be a pure deterministic intersection:

```text
current Tool catalog
  intersect admitted package request
  intersect frozen Capability Grant action/resource selectors
  intersect current revocation/Kill Switch state
  remove unconditional depth-1 forbidden identities
  canonical sort + fingerprint
```

Tool aliases or display names never grant authority. Every call binds the
canonical Tool identity and frozen registry fingerprint. Tool output is
untrusted and cannot add another Tool, Egress rule, Secret ref or approval.

## Project patches

There is no current general Project-file persistence/CAS service suitable for
Agent writes. G20.4 must therefore implement a bounded canonical patch format,
safe path/type/size validation and an executor seam whose Commit takes the
prepared base revision and stable idempotency key. Tests use a deterministic
CAS fake. Production Project mutation remains held until the owning Project
store implements that interface and passes conflict/rollback tests.

Patch rules reject absolute/traversal/NUL, duplicate/Unicode/case collisions,
links/special files, binary ambiguity and unbounded replacement data. Prepare
may validate and fingerprint bytes but performs no mutation. Commit must compare
the exact base revision and cannot silently rebase.

## Artifact boundary

G20.3 already quarantines bytes under Runner ownership. G20.4 may authorize
publication only after current Attempt/grant/budget checks and independent
type/secret/malware policy. Object publication and database attachment use an
object-first then row transaction with compensating object deletion. Artifact
publication is not a Project patch and never implies approval for mutation.

## Recommended package split

Create one `internal/agentbroker` bounded context for Registry, durable effect
coordination and executor interfaces. Extract shared network policy only where
needed to prevent drift. Do not import `agentrunner` into public API/Chat startup
and do not enable Runtime routes in G20.4.
