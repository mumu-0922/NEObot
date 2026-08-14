# agent-runtime-root-canary command design

## Goals

- Keep the production Root canary physically separate from the API and G21.0
  maintenance worker.
- Fail closed before database or Runner use when activation, flags, identity,
  keys, plan, release or role membership drift.
- Supply only the dependencies required by the synthetic G21.1 flow.
- Keep process logs content-free and secret-free.

## Non-goals

- General Root Run scheduling, HTTP/Chat/API activation or user workloads.
- Broker Prepare/Commit, Egress, Secret, Artifact, Provider, MCP, Child, Cron or
  Learning authority.
- Provisioning production credentials, producing activation evidence or
  installing exact-host Runner dependencies.

## Startup flow

```text
strict environment flags and absolute file paths
                    |
                    v
immutable plan + matching Ed25519 key pair
                    |
                    v
exact root_run_canary activation evidence
                    |
                    v
dedicated PostgreSQL LOGIN role proof
                    |
                    v
dedicated mTLS Runner client + signed authority
                    |
                    v
agentrootcanary Service.Run or Service.Health
```

The order is intentional: cheap local validation precedes network access, and
the database role is proven before any service operation receives its handle.

## Design decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Independent command and Compose profile | Reusing `cmd/api` or G21.0 control would collapse activation and identity boundaries. | The profile is default-off and has its own healthcheck and lifecycle. |
| Exact negative feature flags | A canary process must not become an alternate path to general Runtime. | Any broad flag set to true rejects startup. |
| Separate mTLS and database identities | Control-plane maintenance must not silently gain launch authority. | Operators provision distinct files and an exact two-role LOGIN. |
| Match private and public authority keys at startup | Activation binds the public key while launch signing uses the private key. | Key rotation drift fails before issuing a ticket. |
| Sanitize top-level errors | Configuration and downstream errors may contain sensitive paths or credentials. | Logs emit only stable allowlisted summaries. |
| Reuse `agentrootcanary` service | Business recovery and terminalization rules belong outside the process entrypoint. | Command tests focus on wiring, flags, roles and lifecycle. |

## Trust and security boundaries

- Environment and mounted paths are deployment input, not authority. Strict
  parsing, secure-file reads and fingerprint-bound activation establish the
  runtime binding.
- The command accepts only the dedicated canary SPIFFE identity and a private
  HTTPS Runner endpoint validated by the lower activation/client layers.
- The database URL is required to contain a password, but its value is never
  included in returned or logged errors.
- Raw lease tokens remain memory-only inside the orchestrator flow. The command
  neither persists nor reconstructs them.
- The Runner caller policy denies `prepare` and `commit`; the worker receives no
  Broker, Provider, object-store, Redis, MCP or vault credentials.

## Failure, shutdown and rollback

- Invalid configuration, activation or role membership stops startup before a
  canary cycle.
- `SIGINT` and `SIGTERM` cancel the worker context; the service owns exact
  cancel/reconcile and durable recovery behavior.
- A failed healthcheck is non-mutating and never launches a canary.
- Rollback stops only the `agent-runtime-root-canary` profile or sets
  `AGENT_ROOT_RUN_CANARY_ENABLED=false`. G21.0 control remains available for
  reconcile, and durable runtime rows must not be hand-edited or deleted.

## Known limitations

- Successful source and disposable PostgreSQL tests do not prove exact-host
  rootless isolation.
- The command supports one immutable synthetic plan and deliberately requires
  zero final canary inventory.
- Production evidence and credentials remain operator-managed external state.

## Change history

### 2026-08-14 — G21.1 initial implementation

Added the independent Root canary command, strict feature and role gates,
activation/plan/key bindings, mTLS Runner wiring and sanitized lifecycle logs.
