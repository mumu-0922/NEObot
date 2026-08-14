# Immutable Draft provenance and authority

## Comparable patterns

- Package registries separate an immutable uploaded artifact from mutable review
  state; digest identity is established before admission.
- CI systems bind attestations to an exact source revision and artifact digest,
  rather than accepting free-form provenance supplied by the workload.
- Change-review systems use optimistic revision checks and append-only decisions
  so stale reviewers cannot approve a replaced payload.

## Conventions that apply here

- A Draft is a quarantined object plus content-free PostgreSQL metadata. Its
  fingerprint binds the source Run/snapshot, base package, proposed package,
  tests and evidence references.
- The source Run must be terminal `succeeded`, owned by the same user, depth 0,
  and its immutable snapshot must bind the exact base package fingerprint.
- Evidence is bounded to exact source-package and source-Run-event references.
  Modified files require mapped evidence; raw prompts, outputs, secrets and Tool
  bodies never enter Draft rows or audits.
- A Draft package must differ from its base and must not already be admitted.
  Proposal cannot mutate the base package, installation, Run snapshot or Cron
  revision.

## Repository mapping

- Reuse `skillsupply.ValidateArchive` for canonical archive, manifest, inventory,
  package fingerprint and SBOM calculation.
- Store Draft bytes under `skill-drafts/sha256/`; PostgreSQL migration `089`
  stores only object key, fingerprints, bounded inventories and decisions.
- Use a dedicated `agent_learning_control` role with exact function execution;
  Runner, API, Orchestrator, Broker, delegation and Cron roles receive no Draft
  authority.

