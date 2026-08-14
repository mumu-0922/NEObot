# agentbrokerrelay

`agentbrokerrelay` is the private G21.2 mTLS transport between `neo-runnerd`
and the standalone Broker canary. It forwards only Runner `prepare` and
`commit` messages and independently verifies the original signed authority
ticket before invoking the plan-backed Broker target.

## Responsibilities

- Expose one fixed HTTPS path and two protocol methods.
- Require the exact Runner relay client certificate identity.
- Reverify caller, Runner, Attempt, nonce, request fingerprint, snapshot and
  ticket expiry from the original Ed25519 authority ticket.
- Bound request size and concurrency and emit only stable error codes.
- Preserve `outcome_unknown` as non-retryable.

## Usage

The Runner side constructs `NewClient` with an exact private literal endpoint,
dedicated client certificate, key, CA, server name and relay identity. The
Broker process constructs `NewHandler` with a plan-backed `Target`, authority
verifier and the distinct transport/canary identities.

The package is not a public API and must not be mounted into a Sandbox.

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/agentbrokerrelay ./cmd/neo-runnerd
```

See [`DESIGN.md`](./DESIGN.md) for the independent-verification boundary.
