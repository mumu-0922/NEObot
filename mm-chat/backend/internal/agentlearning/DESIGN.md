# Agent Learning Design

## Goals and non-goals

The module allows a completed root Run to propose a bounded Skill improvement
without turning model output or evaluator scores into execution authority. The
decisive invariant is:

```text
Run output -> immutable quarantine Draft -> three fenced checks
           -> human review -> new immutable admitted package
```

It does not self-install a Skill, modify an installation or Grant, expand
runtime authority, run a package in the backend process, expose a public API,
or enable the production Runtime/Scheduler/Learning worker. G20.8 owns product
surfaces and shadow execution; G20.9 owns legacy text-Skill deletion.

## Components and flow

```text
succeeded depth-0 Run + exact snapshot + base package
                    |
                    v
Service.Propose -> archive/authority/evidence/tests validation
                    |
                    +-> skill-drafts/sha256/<archive>.zip
                    |
                    +-> agent_learning_drafts + audit
                              |
                     generation claim
                              v
                  static -> isolation -> evaluation
                              |
                    reviewable | check_failed
                              |
                configured human administrator
                     Reject / Promote
                              |
             canonical objects before SQL transaction
                              |
        admitted learning candidate + immutable package version
                              |
             delayed object-before-row cleanup
```

- `archive.go` owns domain-separated canonical fingerprints, exact evidence and
  test inventory, authority comparison, and bounded ephemeral diffs.
- `static_checker.go` rejects high-confidence prompt override, secret-copy,
  source-laundering, and evaluation-gaming patterns.
- `service.go` owns orchestration, object verification, default-off behavior,
  administrator checks, and injected checker boundaries.
- `repository_postgres.go` is a narrow caller of migration `089`
  `SECURITY DEFINER` functions; PostgreSQL owns transitions and claims.

## State and fencing

```text
quarantined -> checking -> reviewable -> promoted
                        \-> check_failed
reviewable/quarantined/check_failed -> rejected
```

Each check claim increments `check_generation` and binds owner plus expiry.
Only the exact live owner/generation may complete or release it. A policy
failure is terminal for that Draft; correction requires a new Draft
fingerprint. Infrastructure failure is retried at most three times. Cleanup
uses an independent owner/generation/expiry claim and deletes object bytes
before PostgreSQL acknowledgement.

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Immutable Draft, no in-place edit | Prevent evidence and evaluator receipts from being rebound after review. | Every correction produces a new Draft. |
| Version-only authority envelope | Learning must not be a privilege-escalation path. | Runtime image, entrypoints, dependencies, Tools, capabilities, Egress, Secrets, resources, and limits must be byte-equivalent in authority. |
| Three exact check kinds | Separate syntax/policy, isolation behavior, and quality evidence. | Missing, duplicate, stale, or failed receipts cannot reach `reviewable`. |
| Human-only promotion | Evaluation is evidence, not admission authority. | Only the configured administrator may create the new admitted candidate. |
| Content-addressed immutable objects | Make replay and drift detection deterministic. | Existing mismatched bytes fail with `DRAFT_OBJECT_DRIFT`; they are never overwritten. |
| Object-before-row cleanup | Avoid rows claiming deletion while bytes survive. | Missing objects are idempotent success; other deletion failures remain retryable. |

## Trust boundaries and threats

All package bytes, `SKILL.md`, tests, Run-derived evidence, and evaluator
results are untrusted.

| Threat | Control |
| --- | --- |
| Prompt injection or secret laundering into future Skills | Static policy, sanitized bounded durable documents, and no prompt/input/output/Tool/Workspace bodies in Draft rows. |
| Runtime or capability widening | Full base/proposed manifest comparison plus PostgreSQL fingerprint binding. |
| Evaluation gaming | Test inventory fingerprint, static gaming patterns, exact suite/evidence receipts, and separate human admission. |
| Stale worker publishes after reclaim | Owner/generation/expiry fencing in PostgreSQL. |
| Object replacement between review and Promote | Exact size/hash fetch, full archive validation, base-package fingerprint rebinding, immutable object collision check, and package/SBOM rehash. |
| Automated self-approval | Runtime roles have no Draft table DML or Promote execution; Service also requires the configured administrator UUID. |
| Kill Switch or source authority changes | Promote rechecks the current source Run/snapshot/package and hierarchical Kill Switches inside the SQL transaction. |
| Crash reclaim washes bounded retries | Claim functions accept only ready work; reconciliation counts an expired generation before any replacement claim. |
| Cleanup breaks decision replay | Promoted replay reads the immutable decision/admission link and does not require deleted Draft bytes. |

## Known limitations

- The default isolation/evaluation adapters are deliberately unavailable; no
  exact-host or OCI Draft execution is supplied by G20.7.
- The static checker is a high-confidence deny layer, not a general malware or
  semantic-safety proof.
- The administrator diff is bounded and ephemeral; large or binary files show
  only size and SHA-256 information.
- Canonical promotion objects are written before the database transaction. If
  SQL admission fails, unreferenced content-addressed objects may remain and
  require a future generic orphan sweep; no live authority points at them.

## Rollback

Production rollback keeps migration `089` applied and disables Learning. Read,
audit, claim reconciliation, cleanup, and pruning remain available. The down
migration refuses while any Draft/check/decision/cleanup/audit fact or
`learning` candidate exists. Only a disposable empty database may use the
clean `089 -> 088 -> 089` replay.

## Change history

- 2026-08-14: G20.7 Draft-only learning foundation, migration `089`, strict
  Draft schema, held adapters, administrator Promote, and cleanup authority.
- 2026-08-14: Bound base-object identity during review/Promote, require
  reconcile-before-reclaim, and preserve exact Promote replay after quarantine
  cleanup.
