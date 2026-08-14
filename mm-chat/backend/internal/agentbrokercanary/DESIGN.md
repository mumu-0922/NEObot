# agentbrokercanary design

## Goals

- Prove the first production Runner-to-Broker read and Artifact seam without
  enabling user workloads or general mutable effects.
- Keep every action deterministic, replayable and authority-bound.
- Return only canonical fingerprints and counts; never persist raw arguments,
  file contents or MCP results.
- Reconcile terminal Sandbox state even when a Run already completed before a
  process restart.

## Non-goals

- General Agent scheduling, Chat/API activation or arbitrary Tool execution.
- Credentialed MCP, Provider calls, Egress, Secrets, Project mutation, Child,
  Cron, Learning or Skill installation.
- Producing exact-host evidence on a development host.

## Flow

```text
private plan -> stable action bindings -> deterministic Run + lease
  -> signed Runner launch/heartbeat -> signed Prepare -> private relay
  -> Broker read or Artifact executor -> signed Commit -> durable terminal row
  -> pre-signed Cancel -> Sandbox reconcile -> zero inventory
```

Migration `086` terminalizes one Attempt on Commit, so the plan deliberately
uses one Run per action. Cancel authority is issued before Commit and remains
valid only long enough to remove the terminal Sandbox after Commit.

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Exact five-action plan | A canary must not become a generic execution manifest. | Missing, duplicate or extra actions reject the entire plan. |
| Stable hashed IDs | Restart and replay must address the same durable rows. | Changing action identity or idempotency creates a different plan binding. |
| Frozen Grant/Registry resolver | Relayed request fields are untrusted transport data. | Prepare and Commit must match every plan-derived fingerprint and Attempt field. |
| `openat2` confinement | Lexical path checks cannot prevent symlink or magic-link substitution. | Reads require Linux support, regular files, exact size and exact hash. |
| Content-free receipts | Read results must not leak through Broker state or logs. | Only fingerprints, byte counts and MCP content counts leave executors. |
| Object-before-row Artifact publication | A durable row must never point to a missing newly uploaded object. | Row failure deletes the object; success or rejected content clears quarantine. |
| Terminal ambiguity | A possible send cannot be safely retried. | `outcome_unknown` is terminal and later replay only observes it. |

## Trust and security boundaries

- Plan bytes and filesystem/MCP content are untrusted. Private-file mode,
  strict JSON, exact schemas, bounded inputs and immutable fingerprints are
  required before use.
- Project, Workspace and quarantine traversal uses `openat2` beneath an exact
  absolute root and rejects symlinks, non-regular files and drift.
- MCP is manifest-source, `stdio`, `AuthNone`, exact Tool name, read
  classification and empty credential environment only.
- Artifact scanning operates on the same bounded byte slice whose size and
  SHA-256 were verified before upload. Text must be UTF-8 without NUL; JSON is
  decoded with the strict bounded decoder.
- Database and object-store authority live only in the standalone canary
  process. Runner and Sandbox receive neither credential set.

## Failure and rollback

Any plan, activation, lease, snapshot, Grant, Registry or executor drift fails
closed before widening authority. A stale Attempt cannot reach an executor.
Stopping the dedicated Compose profile disables new canary cycles; durable
rows and objects remain governed by migration `091` and normal backup/restore.
Rollback never hand-edits terminal Runtime or Artifact rows.

## Known limitations

- The canary scanner intentionally supports only bounded `text/plain` and
  `application/json` fixtures; it is not a general malware scanner.
- The controller expects the exact-host Artifact intake path to have produced
  the planned quarantine fixture. Unit tests do not claim that live proof.
- Successful package and PostgreSQL tests do not replace rootless isolation
  acceptance on the release host.

## Change history

### 2026-08-14 — G21.2 initial implementation

Added strict plan bindings, reviewed executors, real Runner relay target,
deterministic controller flow, replay checks and terminal reconciliation.
