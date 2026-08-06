# Production Memory candidate boundary research

## Evidence baseline

The consumed schema-v17 buffered Development run passed all 300 cases and all
quality/safety gates. It is strong input for choosing a production candidate,
but its report intentionally states `policySelected=false` and
`promotionEligible=false`. A separately bound Validation identity is required;
replaying schema-v17 or relabeling its artifacts would destroy evidence
provenance.

## Current code findings

### Product policy is stale relative to the passing candidate

`internal/usermemory/hybrid_shadow.go` defines:

- Development guard policy
  `memory_hybrid_fixed_cloud_candidate_judge_negative_guard_development_v1`
  with `NegativePolicyQueryGuardRequired=true`.
- Product policy `memory_hybrid_fixed_cloud_candidate_judge_production_v1`
  without the guard.

The Development policy is deliberately rejected from Server composition, so
installing it directly would cross a documented authority boundary. The safe
path is a new production policy identity that copies the proven guard behavior.

### Product Judge still uses streaming

`internal/httpserver/server.go` resolves the server-default Provider tuple,
constructs `memoryjudge.NewChatAdapter`, then adds the transport-stable retry
decorator. The passing Development candidate instead uses
`memoryjudge.NewBufferedChatAdapter`, which depends on
`chat.BufferedChatProvider`. Runtime must opt into that adapter explicitly; the
ordinary chat streaming interface must not change.

### Existing Validation proves another candidate

`internal/memorycapture/production_memory_judge_validation.go` binds schema
v15, old production policy v1, streaming adapter, and capture mode
`production_fixed_memory_judge_validation`. It is immutable and previously
consumed. A successor must receive new capture/admission/artifact/report/run/
reader/profile identities and bind the guard plus buffered adapter.

### Existing global flag cannot express a single-account canary

`MEMORY_TOOL_LOOP_ENABLED` controls Server installation and Handler exposure for
all users. `chat.Handler.newMemoryToolRuntime` has request context and can read
`auth.UserFromContext(ctx)`, so it is the narrowest fail-closed admission point.
An exact UUID set passed into the Handler prevents non-canary requests from
offering `search_memory` and therefore prevents their retrieval/Judge work.

Role-based admission is rejected: role names are not an immutable unique-user
binding, and a production owner can legitimately have the ordinary `user`
role. Display name/email matching is also mutable and privacy-sensitive.

## Feasible approaches

### A. Versioned v2 candidate + exact UUID gate (selected)

- Add production policy v2, buffered runtime adapter, schema-v18 Validation,
  and `MEMORY_TOOL_LOOP_CANARY_USER_IDS`.
- Strengths: immutable provenance, exact blast radius, simple global rollback,
  and no migration.
- Cost: touches policy, config, capture, scripts, docs, and runtime together.

### B. Mutate production v1 and keep only the global flag

- Small diff, but historical reports no longer describe the same policy and a
  successful Validation would expose every account at once.
- Rejected because it destroys identity integrity and violates the requested
  single-account boundary.

### C. Reuse Development policy directly in production

- Reuses the exact policy object, but crosses the explicit Development-only
  composition boundary and makes product evidence indistinguishable from
  experiment evidence.
- Rejected because it weakens authority separation.

## Operational boundary

The old production Vault wrapper executes `docker compose build admin` and does
not consistently enforce `--no-build --pull never`. It must not be used as the
template for this live attempt. The schema-v17 wrapper already demonstrates the
safer helper-container pattern.

Success-only rollout must:

1. Resolve exactly one intended existing current-login UUID; ambiguity stops.
2. Record backend/unrelated container IDs, image IDs, health, migration version,
   and Memory relation counts.
3. Protect the active env as mode `0600` and retain the exact live backend image.
4. Render the candidate with the exact image and two gates: global true, UUID
   allowlist containing only the selected account.
5. Recreate only backend using `--no-build --no-deps` and no migration.
6. Verify health, clean startup logs, exact flags, account isolation, stable
   schema/counts, and unrelated-container identity.
7. Restore the protected env/image immediately if any post-change check fails.

## Live evidence rules

- Fake lifecycle first, with network denied and no credential inputs.
- Exactly one complete 100-case live attempt; no automatic rerun.
- Raw credentials and Provider content never enter logs or retained artifacts.
- Aggregate artifacts are retained on metric failure for diagnosis.
- Automatic canary authorization is conjunctive with all quality, safety,
  privacy, cost, cleanup, source, and runtime gates.
- A failure preserves all data and leaves recall disabled.
