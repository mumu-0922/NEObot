# Bug Analysis: Draft reclaim and replay lifecycle gaps

## 1. Root Cause Category

- **Category C — Change propagation failure:** generation fencing was added to
  check and cleanup claims, but the direct claim queries also selected expired
  work without propagating the bounded-attempt transition owned by reconcile.
- **Category D — Test coverage gap:** happy-path Promote replay ran before
  cleanup, so the service's accidental dependency on quarantine bytes was not
  exercised after object deletion.
- **Category E — Implicit assumption:** validating a base archive and comparing
  its authority envelope was treated as equivalent to proving it was the exact
  stored base package; the inventory fingerprint binding was missing.

## 2. Why Earlier Verification Missed It

1. Focused claim tests proved stale-generation denial but stopped after one
   direct reclaim, so they did not show that repeated crashes consumed no retry
   budget.
2. Promote replay was tested immediately, while the quarantine object still
   existed; cleanup and replay were each green in isolation but their lifecycle
   ordering was not tested end to end.
3. Base drift tests covered invalid bytes and authority widening, not a valid
   same-authority archive with a different package inventory fingerprint.
4. The cleanup success path never executed the SQL release branch, leaving a
   PL/pgSQL local/column `attempts` ambiguity latent until the drift-preservation
   negative test forced a retry.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Claim only ready work; route expired claims through bounded reconcile before a new generation. | DONE |
| P0 | Runtime validation | Rebind every revalidated base archive to the stored package fingerprint before diff or Promote. | DONE |
| P0 | Test coverage | Replay Promote after object cleanup and deny mismatched replay inputs. | DONE |
| P1 | Executable spec | Record reconcile-before-reclaim and decision-only replay contracts in backend/operations specs. | DONE |
| P1 | SQL convention | Use non-shadowing retry locals and execute negative release branches in PostgreSQL drills. | DONE |

## 4. Systematic Expansion

- **Similar issues:** every leased queue must keep claim, reclaim and retry
  accounting in one explicit lifecycle; direct expired-row selection is a
  warning sign.
- **Design improvement:** idempotency facts must live longer than disposable
  payload bytes. Replay should depend on immutable decisions/receipts, not on
  retention-managed objects.
- **Process improvement:** lifecycle tests must cross boundaries in order:
  decide -> cleanup -> replay, and claim -> expire -> reconcile -> reclaim ->
  exhaust.

## 5. Knowledge Capture

- [x] Backend Agent Runtime executable contract updated.
- [x] Operations Agent Runtime executable contract updated.
- [x] Module DESIGN threat matrix and change history updated.
- [x] PostgreSQL/service regression coverage added.
