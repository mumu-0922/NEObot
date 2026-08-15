# Agent Runtime product canary worker

`agent-runtime-product-canary` is the default-off G21.6 worker. It claims only
migration-095 product requests for one exact activation, derives the fixed
user-bound execution plan, and composes the durable Orchestrator with the
rootless Runner under
`spiffe://neo-chat/agent-runtime-product-canary`.

The worker accepts no prompt, arguments, package selector, Tool, Egress,
Secret or resource override. Its PostgreSQL LOGIN must inherit exactly
`agent_product_canary_worker`, `agent_orchestrator_runtime`, and
`agent_runner_control`. The API and worker credentials are disjoint.

```bash
agent-runtime-product-canary healthcheck
agent-runtime-product-canary run
```

Both commands fail closed unless the strict product activation, plan, release,
policy, mTLS, authority key and exact database role agree. The development host
remains `ISOLATION_UNAVAILABLE` and must not run this profile.

