# G21.4 activation rollout research

## Existing runtime pattern

G21.0-G21.3 activate one independently evidenced capability at a time. Each
execution stage has a dedicated command, mTLS caller, PostgreSQL LOGIN,
default-off Compose profile, strict plan, activation schema/fixtures, enabled
preflight and an aggregate regression gate. Earlier profiles remain unchanged
and broad `AGENT_RUNTIME_ENABLED` stays false.

## Recommended G21.4 shape

- Add `child_agent_canary` rather than enabling delegation in an existing
  Root/Broker/Project controller.
- Require current G21.0-G21.3 readiness, then set only the Child canary and
  delegation flags true inside the dedicated profile.
- Add a fifth Runner caller with lifecycle methods only: `probe`, `list`,
  `reconcile`, `launch`, `heartbeat`, `cancel`. Do not add Broker relay or
  Prepare/Commit authority.
- Add a tenth LOGIN inheriting exactly `agent_orchestrator_runtime`,
  `agent_runner_control`, and `agent_delegation_control`.
- Bind release head, target, plan, Runner identity, authority key, cleanup and
  live negative checks in a short-lived activation record.

## Rejected alternatives

1. Reuse `agent-runtime-root-canary`: this widens a previously frozen stage and
   obscures independent rollback/evidence.
2. Enable a public delegation API: G21.4 must prove the exact runtime seam
   before user/package-controlled child requests exist.
3. Give the Child controller Broker roles: the synthetic Child performs no
   effect, so those credentials and methods have no purpose.

## Affected contracts

- `backend/internal/agentactivation`
- `backend/cmd/neo-runnerd`
- new `backend/internal/agentchildcanary`
- new `backend/cmd/agent-runtime-child-canary`
- Compose, preflight, Phase 0, G21 tracking and Agent Runtime specs

