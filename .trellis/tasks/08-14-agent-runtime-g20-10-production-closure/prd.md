# Agent Runtime G20.10 — Fail-closed production closure

## Goal

Close the Agent Runtime operational contract with executable, content-free
metrics/alert, capacity/budget, retention, backup/restore, disaster-recovery,
incident, rotation, reconciliation, cleanup and promotion-evidence artifacts,
while honestly holding production execution because this exact host remains
`ISOLATION_UNAVAILABLE`.

## Shared baseline

- G20.1-G20.9 contracts, migrations `083`-`090`, Package Skill authority and
  legacy text-Skill hard retirement remain binding.
- The user approved all recommended decisions and direct commits. No additional
  question or sub-Agent is allowed or required.
- Package Skills are the sole eligible Skill execution domain. Assistants and
  MCP Tools remain separate. No browser/API/rootful fallback may be added.
- Current-host runtime truth outranks checked-in plans: exact production
  isolation, live canary, reboot and restore evidence are unavailable here and
  must not be fabricated.
- Protected runtime paths and the live environment file remain untouched.

## Requirements

### Freeze conservative operations policy

- Add a strict versioned policy with single-server capacity, root/Child/Cron
  budget defaults, canary limits, bounded cleanup batches, retention periods,
  metrics and alert thresholds.
- Defaults remain below hard Grant limits and never widen an exact frozen Run,
  Grant, Cron or Child budget. Saturation queues or denies; it never switches to
  another executor.
- Diagnostics permit bounded outcome/reason/scope/mode/runtime-class labels
  only. They exclude users, IDs, fingerprints, paths, URLs, object keys, prompt/
  Tool/Workspace/Skill/Artifact content, credentials, tokens and Secret values.

### Add strict production-closure evidence contract

- Add a closed JSON Schema and fixtures for one content-free record binding the
  release commit, migration head `090`, exact Runner manifest, Runtime Bundle
  and operations-policy fingerprints.
- Require the full live matrix: exact-host isolation, clean-copy, restart/host
  reboot, paired backup/restore, disaster recovery, rollback/forward-fix, Kill
  Switches, credential/runtime rotation, orphan reconciliation,
  `outcome_unknown`, metrics/alerts, capacity/budgets, bounded canary and final
  cleanup.
- A record is either `template` or `production`; template/offline evidence can
  never promote. Promotion state is derived by the validator, never trusted
  from input.
- Require a short review window, unique complete check set, exact fingerprint
  format, passing results and zero temporary canary/Draft/Artifact/Run/Sandbox
  residue while retaining the sanitized promotion record.

### Build fail-closed evaluator and self-test

- Add a read-only evaluator that validates policy/record structure and semantic
  bindings, emits one deterministic content-free JSON decision and uses stable
  exit codes for ready, held and invalid evidence.
- `ISOLATION_UNAVAILABLE` must remain the decisive held reason for the committed
  current-host template. Stale, incomplete, duplicate, drifted or nonzero-
  cleanup records cannot pass.
- Add an offline shell gate that proves valid held, synthetic-ready-but-template,
  malformed, stale, policy-drift and cleanup-residue cases without reading live
  runtime state. An explicit production-record invocation remains nonzero until
  real exact-host evidence is supplied.
- Fold schema/fixture/document anchors into Phase 0 without redefining the
  offline gate as production evidence.

### Close operator runbooks

- Document metrics/alerts, capacity change, on-call severity, isolation drift,
  Kill Switch, orphan/reap, `outcome_unknown`, credential/mTLS/Runtime rotation,
  backup/restore, disaster recovery, rollback/forward-fix and final cleanup.
- `outcome_unknown` permits only exact idempotency-status investigation and an
  append-only operator resolution record; it never permits automatic/new-key
  Commit retry or claims rollback.
- Retention/reconcile/cleanup continues while Runtime is disabled. Unresolved
  ambiguous effects, object drift, failed reap and failed cleanup are retained
  until explicit resolution.
- Cleanup is object/process before row/projection and deletes temporary live
  evidence only after the content-free promotion record is complete.

### Preserve honest held state

- Do not add Compose/startup Runtime, Scheduler, Learning or Shadow execution,
  production adapters, host installation, live data mutation or migration
  `091` merely to manufacture closure.
- Record G20.10 source/operations closure as complete but production promotion
  as held. The Epic is not production-complete until an exact target host
  supplies a `PROMOTION_READY` production record.

## Acceptance criteria

- [x] Policy schema/data freeze capacity, budgets, retention, metrics, alerts,
      canary and cleanup defaults and reject unknown fields.
- [x] Closure schema and evaluator require one exact release-bound complete
      matrix and expose no content/secret/path/URL field.
- [x] Template, `isolation_unavailable`, stale, incomplete, duplicate, drifted
      and residue-bearing evidence cannot produce `PROMOTION_READY`.
- [x] A deterministic synthetic production fixture proves the positive semantic
      path but is visibly test-only and never committed as live evidence.
- [x] Runbooks cover Kill Switch, rotation, orphan/reboot, backup/restore/DR,
      rollback/forward-fix, retention and `outcome_unknown` without manual DML
      or retry ambiguity.
- [x] Phase 0, production-closure self-test, all Agent source/PostgreSQL gates,
      full component checks and full standalone pass.
- [x] Exact-host gate still returns `ISOLATION_UNAVAILABLE` here; no protected
      runtime path/live env is modified.
- [x] Docs/spec/tracking agree that Package Runtime is the only eligible path,
      source operations are closed, and real production promotion remains held.

## Definition of done

- Policy, schemas, fixtures, evaluator, gate, deployment/contract/architecture/
  tracking docs and Trellis specs are synchronized.
- The offline closure verdict is reproducible from a clean copy and cannot be
  mistaken for live promotion evidence.
- Work commit, task archive and journal commits are created without amend/push.

## Technical approach

Use a standard-library, read-only Python evaluator over two strict JSON
documents: a checked-in immutable operations policy and a supplied closure
record. JSON Schema provides portable contract validation; semantic validation
enforces completeness, temporal/release/policy binding, zero residue and
evidence class. A shell self-test generates only temporary synthetic positive
records and verifies every fail-closed branch.

## Decision (ADR-lite)

**Decision:** Close operations and evidence contracts now, but do not enable or
simulate production execution. Promotion requires a separately supplied exact-
host `production` record whose every live check passes.

**Consequences:** The repository gains a deterministic final gate and complete
operator playbook. This development host remains held; the remaining work is
environmental execution of the documented matrix, not a hidden code fallback.

## Out of scope

- Installing/configuring the target host, writing live credentials, enabling
  workers or executing a live package/canary on this host.
- Browser/API/rootful execution, dual legacy execution, Child depth above one,
  mutable canary before read-only promotion or automatic Draft promotion.
- Live deletion of canary/Draft/Artifact/Run state without supplied production
  authority and backup/rollback evidence.
- Unrelated Assistant, MCP, Memory, Knowledge, Chat or provider changes.

## Research references

- [`research/operations-seams.md`](research/operations-seams.md)
- [`research/promotion-evidence.md`](research/promotion-evidence.md)
- [`research/defaults-and-runbooks.md`](research/defaults-and-runbooks.md)
- [`research/current-host-boundary.md`](research/current-host-boundary.md)
