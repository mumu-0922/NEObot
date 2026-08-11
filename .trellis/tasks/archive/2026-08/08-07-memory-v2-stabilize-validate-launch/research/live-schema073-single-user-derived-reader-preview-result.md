# Live schema-073 single-user derived Reader preview result

## Outcome

L2 Scene and L3 Persona Readers are live for the exact sole user under the
separately audited migration-`073` preview authority. This is not formal L2/L3
promotion and did not mutate formal promotion evidence or the L1 retrieval
pointer.

```text
schema=73
users=1
L1 current=2
L1 active_retrieval_profile_id=null
preview events=5, latest enabled=true
L2 current/active/ready=1/1/1
L3 current/active/ready=1/1/1
L2 formal promotion events=0
L3 formal promotion events=0
derived unfinished/dead jobs=0
Memory health=ready, ready/pending/failed=2/0/0
```

The live API and Memory Worker are healthy on:

```text
image=mm-chat/backend:memory-l2-l3-reader-preview-schema073-candidate-20260810t083217z
image ID=sha256:9ee41134bfcf3cf84028bda7cebf852a0fd2915a4a289b51c7b6feef2964a7b2
```

Both shadow flags and both API Reader flags are true. Reader flags are absent
from the Worker environment. PostgreSQL was never restarted, and its container
ID plus every unrelated container ID remained unchanged.

## Verification before live

- Focused migration race tests passed.
- All Backend tests and `go vet` passed.
- Frontend format/lint/type-check, `964/964` tests, and production build passed.
- RAG Ruff/mypy passed; `1906` tests passed and `7` integration tests skipped by
  their explicit external-service gates.
- `verify-standalone.sh --full` passed.
- The exact packaged candidate was run over a data-only restore of a verified
  schema-72 live logical dump on disposable PostgreSQL 17. It proved packaged
  `72 -> 73`, enable, active L2/L3 reconciliation, unchanged null L1 pointer,
  zero formal promotions, append-only disable, and complete teardown.

The RAG package also gained its missing PEP 561 `py.typed` marker after the
repository-mandated bare `uv run mypy` exposed that packaging defect. The clean
standalone copy continued to pass `mypy src`.

## Live backup and migration

The first live attempt used this verified mode-`0600` logical dump:

```text
mm-chat/backup/postgres/postgres-schema072-before-reader-preview-20260810T083252Z.dump
```

The frozen candidate migrator applied `073` exactly once while Backend and the
Memory Worker were stopped. The migrator container image ID matched the frozen
candidate. PostgreSQL remained running on the same container. Migration `073`
is retained; rollback is behavioral and never downs or deletes preview audit.

## Provider-free verifier corrections and rollback proof

Two verifier-only defects were encountered after migration. Neither made a
Provider or Chat request, and both retained content-free audit evidence.

1. The first post-state SQL placed `ORDER BY/LIMIT` directly inside one arm of a
   `UNION`. PostgreSQL rejected the verifier. The initial shell trap copied the
   rollback environment but its state flags had been assigned inside a `tee`
   pipeline subshell, so it did not invoke the database disable/recreation.
   A direct corrective rollback immediately appended the disable event,
   reconciled both derived artifacts to non-active, restored both Reader flags
   false, and recreated only Backend/Worker. This established the rule that
   rollout state used by traps must never be mutated inside a pipeline subshell.
2. The fresh retry correctly rolled back through its trap because it required
   `reason=memory_ready`. The actual endpoint contract returned
   `status=ready`, `readyCount=2`, `pendingCount=0`, and `failedCount=0`, with an
   optional absent reason. The final verifier binds those authoritative fields
   instead of inventing a required reason.

Fresh verified schema-73 backups were taken after each rollback. The final
retry backup is:

```text
mm-chat/backup/postgres/postgres-schema073-before-reader-preview-final-retry-20260810T090329Z.dump
```

The five immutable events therefore represent:

```text
enable -> verifier disable -> retry enable -> verifier disable -> final enable
```

This is expected audit history, not five promotions or repeated migration.
Formal promotion event counts stayed zero throughout.

## Final evidence and rollback

Private evidence is retained mode `0700`, with files mode `0600`, under:

```text
/var/tmp/neo-chat-memory-l2-l3-reader-preview-schema073-live-final-retry-20260810T090329Z
```

The final API checks proved `/ready`, `/v1/me`, `/v1/memory-health`, and
`/v1/memory-governance`. Governance reported both derived profiles active with
one Scene and one Persona. A later observation interval repeated healthy
Backend/Worker, ready Memory health, active L2/L3, null L1 pointer, and zero
formal promotions.

Operational rollback is already exercised: append one fresh disable event with
an uppercase bounded reason, set both Reader flags false, and force-recreate
only Backend/Worker on the same schema-73 candidate. Adding a second user also
appends a database disable automatically; deleting that user never re-enables
preview.

## Debug retrospective

### Root-cause categories

- **D — Test coverage gap**: the exact final verifier SQL and rollback trap were
  not failure-injected together before the first live attempt.
- **E — Implicit assumption**: pipeline assignment was assumed to mutate the
  parent shell, PostgreSQL set-operation clause ownership was assumed, and an
  optional API field was assumed mandatory.
- **B — Cross-layer contract**: the verifier asserted a response field outside
  the actual Backend health contract.

### Why the first corrections failed

1. The invalid SQL stopped post-state verification, while the `tee` pipeline
   hid the phase mutation from the parent-shell `EXIT` trap. Corrective rollback
   therefore had to be invoked directly before any retry.
2. The next verifier fixed rollback control but still encoded an invented
   `reason=memory_ready` requirement. Its now-correct trap safely disabled the
   preview, proving rollback while exposing the API contract mismatch.

### Prevention mechanisms

| Priority | Mechanism | Action | Status |
| --- | --- | --- | --- |
| P0 | Runtime | Keep trap phase state in the parent shell and inject one pre-live failure per phase. | Done in operations spec |
| P0 | Test | Execute exact final verifier SQL against the disposable rehearsal database. | Done in operations spec |
| P0 | Contract | Assert documented health status/counts and replay payloads with optional fields absent. | Done in operations spec |
| P1 | Review | Reject unparenthesized ordered/limited `UNION` arms and pipeline-owned rollback state. | Done in operations spec |

### Systematic expansion and capture

Future live scripts must audit every pipeline feeding an `EXIT` trap, run the
exact verifier rather than a hand-simplified surrogate during rehearsal, and
derive required API fields from current source plus captured runtime behavior.
These contracts are captured in
`.trellis/spec/operations/runtime-recreate-image-pinning.md`; the launch and
rollback history is captured in `mm-chat/docs/tracking/process.md`.

## Frontend copy cleanup

The owner subsequently removed the generic derived-content notice below each
L2 Scene and L3 Persona profile header. The status/generation/readiness badges,
rebuild actions, per-artifact controls, and underlying Reader behavior were not
changed. The component and all three locale catalogs no longer contain the two
notice keys.

Focused composition coverage, targeted Prettier, ESLint, strict TypeScript,
and the production build passed. The live rollout recreated only Frontend on:

```text
image=mm-chat/frontend:memory-governance-derived-copy-cleanup-20260810T093913Z
image ID=sha256:9adcda09415856333412c50729eeb114381ddf1e10aed4ea6f1a7a1e0ea911f4
```

Frontend remained healthy with HTTP `200`; the built image contains neither
notice. Backend, Memory Worker, PostgreSQL, MinIO, and Redis container IDs were
unchanged. Private rollout evidence is retained under
`/var/tmp/neo-chat-memory-governance-derived-copy-cleanup-20260810T093913Z`.

## Frontend bounded governance lists

Deletion progress and search diagnostics now retain all returned records but
bound their keyboard-scrollable viewports to six minimum-height cards.
Conversation Use/Learn policies likewise retain all conversations and bound
the viewport to eight cards. Each region reserves a stable right-side
scrollbar gutter and preserves its headings, controls, and empty states.

Focused composition coverage, targeted formatting, ESLint, strict TypeScript,
and the production build passed; the owner explicitly excluded the unrelated
full test suite. The first frontend replacement correctly rolled back because
its verifier searched minified assets for source Tailwind class names instead
of compiled CSS declarations. The corrected retry proved the compiled
`max-height:39.75rem`, `max-height:51.5rem`, and
`scrollbar-gutter:stable` rules, then recreated only Frontend on:

```text
image=mm-chat/frontend:memory-governance-bounded-scroll-20260811T005030Z
image ID=sha256:92486df8cfab806faf4689cfd2638bda48a38b74cd4855ac20ce3e2b8fe02985
```

Frontend is healthy with HTTP `200`, Memory health is ready at `3/0/0`, and
Backend, Memory Worker, PostgreSQL, MinIO, and Redis container IDs remained
unchanged. Successful private evidence is retained under
`/var/tmp/neo-chat-memory-governance-bounded-scroll-retry-20260811T005314Z`.
