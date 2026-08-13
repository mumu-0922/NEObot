# Runner RPC, mTLS and replay authority

## Boundary

The existing `neo.runner-rpc/v1` JSON Schema is the wire authority. G20.3 owns
`probe`, `launch`, `heartbeat`, `cancel`, and reconciliation/listing. G20.4 owns
`prepare` and `commit`; the G20.3 daemon must reject them as unavailable rather
than grow a partial side-effect path.

## Recommended transport

- HTTPS with mutual TLS on one explicitly configured private listen address.
- TLS 1.3 only for the new control plane; exact client certificate identity is
  allowlisted by the Runner.
- The server refuses root execution, missing certificate files, plaintext
  listeners, wildcard/public addresses and certificates without the expected
  service identity.
- The Backend-facing client uses its own certificate and pinned CA; no bearer
  token fallback exists.
- Request body, header, deadline and concurrency limits apply before decode.
- Strict JSON decode rejects duplicate/unknown fields and method/body mismatch.

A Unix socket can reduce exposure but does not itself supply the contract's
service identity. The initial release uses loopback/private TCP plus mandatory
mTLS so both authentication and deployment reachability are testable. A future
Unix-socket TLS transport may be added without changing the envelope.

## Durable replay fence

PostgreSQL remains the control-plane authority, but the deployment contract
forbids giving `neo-runnerd` a database credential. Replay therefore has two
independent durable fences:

1. the Control Plane claims the request and persists an exact authority-ticket
   record in PostgreSQL before signing the short-lived ticket;
2. `neo-runnerd` validates that signature and claims the same request in a
   mode-0600 host-local fsync ledger before any driver action.

Both are keyed by:

```text
caller certificate identity + requestId + nonce
```

The PostgreSQL claim function and local ledger atomically:

1. validates bounded identifiers/fingerprints and the method;
2. rejects requests older/newer than the configured clock window;
3. inserts one request fingerprint and expiry;
4. returns exact replay for a duplicate key/fingerprint only when an already
   persisted terminal response exists;
5. rejects duplicate-inflight and key/fingerprint mismatch as
   `REPLAY_DETECTED`;
6. stores only a bounded sanitized response envelope, never lease tokens,
   content, host paths, stdout/stderr or secrets.

An ordinary Backend or daemon restart therefore cannot reopen the nonce
acceptance window. The local ledger is not Run authority and cannot mint a
ticket. Cleanup of expired terminal replay rows remains callable while Runtime
execution is disabled; local compaction retains all live/inflight claims and
fails closed rather than discarding them.

## Lease and launch fencing

Runner RPC replay protection is not Run authority. Before signing, the Control
Plane binds exact Run, Step, Attempt, lease generation, owner, token and
snapshot fingerprint to PostgreSQL G20.2 authority. The signed ticket carries
only the token digest and a bounded expiry. Runner recomputes that digest from
the envelope and verifies the signature/bindings before the OCI driver is
called. A higher generation or expired token cannot receive a new valid ticket.
The Runner stores only the token digest in its local Sandbox record and never
returns/logs the token.

## Response/recovery rule

- A durable response is recorded before it is returned.
- Launch recovery reconciles a deterministic Sandbox name derived from exact
  Attempt identity. It never launches a second Sandbox for a replay.
- `list` returns bounded content-free Sandbox descriptors for Orchestrator
  reconciliation.
- Unknown, stale or snapshot-mismatched Sandboxes are killed and cleaned.
- A Runner process crash after OCI create but before durable response is
  resolved by exact Sandbox inspection, not by blind create retry.
