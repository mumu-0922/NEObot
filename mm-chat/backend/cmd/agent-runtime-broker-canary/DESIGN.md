# agent-runtime-broker-canary command design

## Goals

- Compose G21.2 dependencies without widening API, control, Root-canary or
  Runner authority.
- Reject incomplete or shared identities, credentials and activation before a
  canary cycle.
- Keep top-level logs content-free and credential-free.

## Non-goals

- General Agent Runtime activation or user-triggered execution.
- Credential provisioning, bucket creation or exact-host installation.
- Passing database, object-store, MCP or relay private-key material to Runner
  or Sandbox.

## Startup flow

```text
strict flags, URLs, paths, timeouts and identities
  -> private plan + matching authority key pair
  -> exact broker_artifact_canary activation evidence
  -> exact four-role PostgreSQL LOGIN proof
  -> existing S3-compatible bucket + private quarantine
  -> reviewed executors + durable Broker
  -> private mTLS relay + deterministic canary controller
```

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Independent process/profile | Other Agent services have different reviewed method sets. | G21.2 remains default-off and independently reversible. |
| Exact negative broad flags | A synthetic process must not become a hidden general Runtime entrypoint. | Any Runtime, Scheduler, Skill, Learning, Child or generic Broker flag rejects startup. |
| Literal-private exact relay URL | DNS or wildcard binding would widen the transport surface. | Listen address and activation-bound endpoint must be the same private IP and port. |
| Existing bucket only | Bucket creation is an operational privilege, not canary authority. | `S3_BUCKET_AUTO_CREATE` must remain false and readiness fails if absent. |
| Exact four-role LOGIN | The controller needs Orchestrator, Runner, Broker and Artifact functions but no owner role. | Recursive membership drift or table DML fails startup. |
| Short RPC and authority TTL | Pre-signed cleanup authority must not outlive the terminalization window. | RPC timeout is at most 10 seconds and authority TTL is 10–15 seconds. |

## Security boundary

Mounted configuration is untrusted deployment input. Strict parsing precedes
network access; activation binds release, target, policy, plan, keys, Runner and
relay trust tuples. Relay transport and original operation authority use
different identities. Configuration errors are sanitized before logging.

The process owns only the credentials required by its narrow responsibilities:
the dedicated database LOGIN, object store, MCP Runner token, authority private
key and relay server key. Compose must keep every one out of `neo-runnerd` and
Sandbox configuration.

## Failure, shutdown and rollback

Invalid configuration, activation, role membership, object readiness or mTLS
material stops startup. `SIGINT`/`SIGTERM` cancel the controller and shut down
the relay with a bounded timeout. Rollback stops this one profile or sets its
dedicated flag false; durable Runtime and Artifact rows are not deleted.

## Known limitations

- `healthcheck` requires all external dependencies and therefore intentionally
  fails while the profile is not fully activated.
- Exact-host Artifact intake and rootless isolation remain separate release
  evidence; development-host tests must continue to report
  `ISOLATION_UNAVAILABLE`.

## Change history

### 2026-08-14 — G21.2 initial implementation

Added strict startup configuration, exact database-role proof, object/MCP
wiring, private relay server and deterministic controller lifecycle.
