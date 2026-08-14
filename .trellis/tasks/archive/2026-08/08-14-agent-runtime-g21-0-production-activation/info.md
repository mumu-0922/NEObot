# G21.0 technical design index

## Delivery slices

1. Strict staged activation schema/evaluator/fixtures and runtime verifier.
2. Outbound Runner request constructor and control-only maintenance worker.
3. Dedicated command/image boundary plus default-off Compose/preflight wiring.
4. Content-addressed exact-host bundle builder/verifier and systemd hardening.
5. Focused gate, full verification and architecture/contract/deployment/tracking/spec sync.

## Runtime truth

```text
current development host: ISOLATION_UNAVAILABLE
Agent execution: disabled
control worker: default off
production Runner in Compose: forbidden
G21.0 allowed RPC: probe, list, reconcile
G21.0 denied RPC: launch, heartbeat, cancel, prepare, commit
```

## Rollback

Keep all Agent flags false or stop the `agent-runtime-control` profile. This
removes every new dial/worker path without changing migration `090` authority.
The host Runner remains independently stoppable by systemd; no application
Compose rollback deletes Runner state or evidence.
