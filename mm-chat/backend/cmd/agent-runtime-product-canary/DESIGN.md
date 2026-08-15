# Product canary worker design

## Flow

```text
authenticated opt-in/cohort user
  -> API migration-095 enqueue function
  -> leased product request
  -> dedicated worker
  -> durable Orchestrator Run/Attempt
  -> signed rootless Runner lifecycle
  -> exact cancel/reap and terminal chain
  -> immutable product receipt
```

The API can append the fixed request but cannot claim or execute it. The worker
can claim and complete requests but cannot provision/disable activation, change
Shadow policy, or record final promotion. The operator promotion function is a
third authority and binds the external `PROMOTION_READY` closure fingerprint.

The Runner method set is `probe/list/reconcile/launch/heartbeat/cancel`. There
is no Broker relay and no `prepare`, `commit`, or `result` authority. A restart
reuses the request-derived idempotency key and resolves the existing durable
Attempt before any replacement.

