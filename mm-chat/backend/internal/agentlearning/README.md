# Agent Learning

`agentlearning` implements the held G20.7 Draft-only learning control plane. A
successful root Agent Run may propose a version-only Skill package revision,
but the package remains quarantined until three exact checks pass and the
configured administrator explicitly promotes it.

The package is intentionally internal. G20.7 adds no HTTP handler, frontend,
Chat hook, startup worker, Compose service, automatic installation, or
production package executor.

## Responsibilities

- revalidate the exact base and proposed Skill archives through
  `skillsupply.ValidateArchive` and bind the base bytes back to the stored
  package fingerprint;
- reject runtime, entrypoint, dependency, Tool, capability, Egress, Secret, or
  resource authority changes;
- derive canonical Draft, evidence, test, archive, SBOM, and package
  fingerprints;
- store immutable content-addressed quarantine objects without overwriting a
  mismatched object;
- run `static`, injected `isolation`, and injected `evaluation` checks under a
  generation-fenced PostgreSQL claim;
- expose a bounded in-memory administrator diff;
- perform administrator-only Reject/Promote decisions, preserve exact decision
  replay after quarantine retention, and verify object fingerprints before
  object-before-row cleanup; and
- route expired claims through bounded reconciliation and keep reconciliation/
  cleanup available while Learning execution is off.

## Construction

```go
service := agentlearning.NewService(
    agentlearning.WithRepository(agentlearning.NewPostgresRepository(database)),
    agentlearning.WithObjectStore(objects),
    agentlearning.WithAdministratorUserID(administratorUserID),
    agentlearning.WithLearningEnabled(false),
)
```

The default is fail closed:

- Learning is disabled until `WithLearningEnabled(true)` is supplied.
- Isolation and evaluation adapters return `CHECK_UNAVAILABLE` until explicit
  implementations are injected with `WithCheckers`.
- No constructor in backend startup currently enables or wires this service.

## Main API

| Method | Contract |
| --- | --- |
| `Propose` | Accept a same-user succeeded depth-0 Run and persist one immutable quarantined Draft. |
| `RunChecks` | Claim Drafts and publish exactly one static/isolation/evaluation bundle for the fenced generation. |
| `GetDiff` | Hydrate bounded base/Draft text or binary summaries in memory for the configured administrator. |
| `Reject` | Append an administrator rejection and schedule quarantine cleanup. |
| `Promote` | Rehash and revalidate exact bytes, then atomically create a new admitted `learning` candidate/package. |
| `Cleanup` | Verify exact quarantine bytes, preserve drift for retry/incident handling, then delete before acknowledging the cleanup row. |
| `Reconcile` / `Prune` | Reclaim expired claims and remove bounded terminal history. |

## Persistence and verification

- PostgreSQL authority: migration `089_agent_draft_learning`.
- Object prefixes: `skill-drafts/sha256/`, `skill-quarantine/sha256/`,
  `skill-packages/sha256/`, and `skill-sboms/sha256/`.
- Contract schema: `docs/contracts/schemas/neo-skill-draft.schema.json`.
- Source gate: `bash mm-chat/scripts/verify-agent-learning.sh`.
- PostgreSQL 17 gate:
  `bash mm-chat/scripts/verify-agent-learning-postgres17.sh`.

See [DESIGN.md](DESIGN.md) for state, threat, and rollback details.
