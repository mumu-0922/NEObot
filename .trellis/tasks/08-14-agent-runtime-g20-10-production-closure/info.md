# G20.10 technical design index

## Delivery slices

1. Versioned operations policy and strict policy schema.
2. Strict content-free production-closure evidence schema and held fixtures.
3. Read-only semantic evaluator plus offline fail-closed self-test.
4. On-call, incident, rotation, retention, backup/restore, DR and cleanup
   runbooks.
5. Phase 0 and documentation/spec/tracking synchronization.

## Runtime state

```text
legacy pure-text executor: deleted
Package Skill Runtime authority: sole eligible domain
operations/source closure: implemented by G20.10
exact-host production promotion: ISOLATION_UNAVAILABLE
fallback executor: forbidden/absent
```

No migration or startup wiring is required for this operations-only slice.
Actual promotion evidence is generated outside Git on the exact production
host and reviewed through the read-only gate.

## Verification evidence

- Production-closure self-test passed positive, isolation/template held, stale,
  residue, policy drift, missing/duplicate check, malformed, writable-file and
  symlink cases.
- Phase 0 passed schemas, fixtures, policy hash binding, docs/links, product/
  Shadow and fail-closed routes from an isolated copy.
- Every Agent source gate and every PostgreSQL 17 gate from Skill supply through
  G20.9 passed at migration head `090`, including dump/restore and guarded
  down/up drills.
- Full standalone passed: frontend 188 files / 909 tests plus build, backend vet
  and all tests, RAG 1906 passed / 7 skipped plus Ruff/mypy.
- The exact-host probe remained nonzero with
  `Agent Runner host verification: ISOLATION_UNAVAILABLE`; no live promotion was
  claimed.

## Verification repair

The full source matrix exposed a stale G20.7 assertion that banned all imports
of `internal/agentlearning`. G20.8 intentionally added only administrator Review
wiring. The gate now allowlists the exact Agent Control/API importers, requires
`WithLearningEnabled(false)`, and rejects startup Claim/Reconcile/Cleanup/Prune
calls instead of reverting the product facade.
