# agent-runtime-project-canary command design

## Goals

- Compose one independently reversible G21.3 Project CAS without widening API,
  control, Root, Broker/Artifact or Runner authority.
- Reject flag, identity, relay, key, plan, approval and principal drift before
  database or Runner access.
- Recover acknowledgement loss from durable Project status and never issue a
  second CAS after a possible send.

## Startup flow

```text
strict flags/paths/literal-private relay/time bounds
  -> strict one-action plan + distinct authority/approval/TLS keys
  -> stable activation binding + signed approval verification
  -> project_mutation_canary activation evidence at head 092
  -> exact four-role function-only LOGIN proof
  -> Project CAS/status/cleanup executor + private mTLS relay
  -> controller cycle or read-only health
```

The stable activation binding is domain-separated over stage, release, target,
Runner, plan, caller and relay endpoint. It excludes the raw activation-record
SHA because that record binds the approval document; including both hashes in
each other would create an impossible cycle. Approval also does not pre-bind a
random Broker Intent. Exact Prepare runs first, then the verified document is
appended under one fixed approval ID; durable collision fences prevent reuse.

## Security decisions

| Decision | Consequence |
| --- | --- |
| Dedicated caller and relay | Broker and Project traffic cannot cross-route. |
| Offline approval private key | The controller cannot approve its own mutation. |
| Synthetic resource provisioned by operator | Runtime cannot create or select arbitrary Projects. |
| Function-only ninth LOGIN | CAS/status/cleanup are available without table DML or owner rights. |
| Atomic Project CAS plus receipt | After-send acknowledgement loss is decidable after restart. |
| Baseline cleanup after Broker commit | Synthetic content is restored without deleting immutable audit facts. |

## Failure semantics

- Rejection before Commit creates no Project write.
- Stale lease/generation, Grant revocation or Kill Switch drift rejects inside
  the SQL CAS fence and creates no receipt.
- Matching receipt means committed; absent receipt plus unchanged clean base
  means not sent; every conflicting or unavailable state becomes terminal
  `outcome_unknown`.
- Terminal restart may reconcile cleanup only. It never prepares or commits a
  second mutation under another key.
- Cleanup failure reports recovery pending and keeps the profile unavailable
  until the exact baseline is restored.

## Rollback

Disable only the Project canary flag/profile. Keep migration `092` and its
immutable facts. A destructive migration down is operator-only after evidence
archive and a joint truncate of all three synthetic Project tables.

## Current hold

Source, Compose and disposable PostgreSQL gates are not exact-host activation.
This development host remains `ISOLATION_UNAVAILABLE`; user Projects, MCP
writes and general Agent execution remain disabled.
