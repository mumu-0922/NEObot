# Agent Runtime Child Canary Command

This standalone command runs or health-checks the default-off G21.4 depth-one
Child Agent canary.

```text
mm-chat-agent-runtime-child-canary run
mm-chat-agent-runtime-child-canary healthcheck
```

Startup requires G21.0-G21.3 flags plus `AGENT_DELEGATION_ENABLED=true` and
`AGENT_CHILD_CANARY_ENABLED=true`. Broad Runtime, Broker read/mutation,
Scheduler, Skill install and Learning flags must remain false.

The process requires a dedicated mTLS identity
`spiffe://neo-chat/agent-runtime-child-canary`, a dedicated LOGIN inheriting
exactly three NOLOGIN capability roles, private plan/evidence/key files and a
current migration-093 activation record. It does not configure a Broker relay.
