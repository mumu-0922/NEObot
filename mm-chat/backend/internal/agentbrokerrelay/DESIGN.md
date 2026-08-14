# agentbrokerrelay design

## Goals

- Connect the already verified Runner ingress to durable Broker authority
  without putting PostgreSQL, object-store or MCP credentials in Runner.
- Make transport identity and original operation authority independent checks.
- Keep the protocol closed to every method except `prepare` and `commit`.

## Non-goals

- General RPC routing, service discovery, public ingress or browser access.
- Launch, heartbeat, cancel, probe, list or reconcile forwarding.
- Trusting the Runner merely because its mTLS certificate passed.

## Architecture

```text
Broker canary caller --signed request--> neo-runnerd
  neo-runnerd verifies authority and method policy
      --private mTLS prepare/commit--> relay Handler
  Handler verifies Runner transport identity
  Handler independently verifies original signed authority
      --> plan-backed Target --> durable Broker
```

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Separate Runner relay identity | Reusing the canary caller would collapse caller and transport trust. | The two certificate identities must differ exactly. |
| Preserve the full authority ticket | Re-signing at Runner would hide the original caller binding. | Relay can verify the same nonce, request and Attempt independently. |
| One path and two methods | A general proxy would silently widen Runner authority. | Unknown versions/methods and all other paths fail closed. |
| No automatic retry hint | Commit may have reached an executor before acknowledgement loss. | All relay errors are non-retryable; `outcome_unknown` stays terminal. |
| Bounded body and concurrency | The private network is not a trust boundary. | Oversized or overloaded requests fail before decoding or Broker access. |

## Security boundary

The TLS verifier must produce exactly one verified chain whose leaf common name
matches the configured Runner relay SPIFFE identity. This transport proof is
necessary but insufficient: the handler also reconstructs the unsigned Runner
request fingerprint and verifies the original Ed25519 authority ticket against
the exact canary identity, Runner ID, method, Attempt and snapshot.

Responses contain only bounded Runner result shapes or stable error codes.
Database errors, credentials, plan contents and executor results are never
forwarded verbatim.

## Failure and rollback

Any TLS, version, method, body, ticket or target mismatch is fail-closed.
Disabling the G21.2 profile removes the listener; `neo-runnerd` falls back to
its unavailable relay unless the complete dedicated configuration is present.

## Known limitations

- Certificate identity currently uses the verified leaf common name because
  that is the established Runner mTLS contract in this repository.
- The relay is intentionally single-target and has no discovery or failover.

## Change history

### 2026-08-14 — G21.2 initial implementation

Added the strict mTLS client/server protocol and independent authority replay.
