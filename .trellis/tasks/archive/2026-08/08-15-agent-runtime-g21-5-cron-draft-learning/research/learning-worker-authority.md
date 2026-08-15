# G21.5 Draft-learning worker authority research

## Question

How should check/cleanup authority be split from human review and Promote?

## Repository findings

- Migration 089 grants `agent_learning_control` every operation: create Draft,
  check claim/complete/release, Reject, Promote, cleanup, reconcile and prune,
  plus SELECT on all learning tables.
- `agentlearning.Service` correctly requires a configured administrator for
  Reject/Promote, but database authorization is the decisive boundary. A worker
  inheriting `agent_learning_control` could call `agent_learning_promote`
  directly and bypass the Go administrator check.
- `agent_learning_claim_checks`, `agent_learning_claim_cleanup`, reconcile and
  prune are global. A G21.5 worker would silently process all existing Drafts,
  which is a product cohort and belongs to G21.6.
- Promotion already revalidates provenance, source Run/snapshot, package bytes,
  unchanged authority, all three check results and Kill Switch state. Those
  controls should remain on a separate human/operator path.

## Comparable patterns

- CI systems separate executor credentials from protected-environment approval;
  a job runner produces evidence but cannot approve deployment.
- Package registries use quarantine scanners that can attach attestations while
  publisher/reviewer identities retain promotion authority.
- Kubernetes controllers commonly use distinct service accounts for reconcilers
  and admission operators, even when both manipulate the same resource family.

## Recommended split

Migration 094 adds an operator-provisioned exact Draft activation target and a
new `agent_learning_worker` NOLOGIN role. It may execute only target-scoped:

- check claim, check complete and check release;
- Runner-check attempt begin/authority/result/failure/cleanup;
- quarantine object cleanup claim/complete/release;
- expired-claim reconcile and terminal retention prune.

It receives no Draft proposal, diff/review, Reject, Promote, package-candidate
insert, direct DML, generic `agent_learning_control`, or owner membership.
The exact-host LOGIN inherits only `agent_learning_worker`.

Human Promote remains a separate, explicit administrator action through the
existing reviewed path. Activation evidence binds the resulting decision ID,
actor class, Draft/proposed package fingerprint and cleanup result without
placing the administrator credential in the worker.

## Alternatives rejected

- **Reuse `agent_learning_control`:** simplest wiring, but worker compromise
  becomes autonomous package admission.
- **Rely on Go administrator checks:** SQL can still be invoked directly by the
  inherited role.
- **Create a worker-owned intermediate package:** would turn check output into
  execution/admission authority and violate Draft-only learning.

## Required proof

- Worker LOGIN recursively inherits exactly one narrow role and cannot `SET
  ROLE` to owner/control roles.
- Direct SELECT/DML, Propose, Reject and Promote fail.
- Exact bound Draft can progress through checks and cleanup; another quarantined
  Draft remains untouched.
- A separate administrator can Promote only after all receipts pass; worker
  restart cannot mint the human decision.
