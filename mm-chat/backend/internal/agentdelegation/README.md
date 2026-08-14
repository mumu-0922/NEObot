# Agent delegation

`agentdelegation` is the held G20.5 Backend control foundation for one-level
Child Agents. It derives Child authority from a durable depth-0 Parent, reserves
Parent budgets transactionally through PostgreSQL, validates launch authority,
requires terminal usage settlement, and coordinates Child-first cancel/kill
reaping. Reconciliation discovers terminal, expired, reclaimed or
Kill-Switched Parents before retrying durable failed reap work.

It is intentionally not imported by HTTP, Chat, frontend, or a startup worker.
Passing its tests is not production Runner or exact-host isolation evidence.

## Verification

```bash
bash mm-chat/scripts/verify-agent-delegation.sh
bash mm-chat/scripts/verify-agent-delegation-postgres17.sh
```
