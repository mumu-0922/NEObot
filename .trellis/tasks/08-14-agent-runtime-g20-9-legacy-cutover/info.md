# G20.9 technical design index

## Delivery slices

1. **Browser retirement migration**
   - bump shared persistence version;
   - strip eight Settings fields plus Session/Workspace selections at raw and
     Zustand normalization layers;
   - persist completion only after successful, compensatable writes.

2. **Legacy source/assets deletion**
   - remove editor/sidebar/URL/composer/workspace/store/service/library/static
     catalog and all Chat resolver/context injection;
   - retain package Skill Agent Center domain unchanged.

3. **History projection**
   - collapse old invocation arrays to `legacySkillRetired: true` at API and
     browser normalization boundaries;
   - render one localized, non-interactive retirement fact.

4. **Server state cutover**
   - sanitize retired Conversation metadata on request/read;
   - provide backup/count-confirmed PostgreSQL JSONB-key deletion with no
     pretend reversible schema migration.

5. **Proof and documentation**
   - add focused offline/PostgreSQL gate, negative executable-reference scan,
     reload/restart tests and synchronized Runtime docs/specs.

## Runtime state

```text
legacy pure-text executor: deleted
package Skill control plane: retained
exact-host Runtime: ISOLATION_UNAVAILABLE
fallback executor: forbidden/absent
```

G20.9 source cutover does not promote production. Exact-host isolation,
backup/restore, canary budget and rollback rehearsal remain mandatory.

## Verification evidence

- Frontend format/lint/typecheck, 188 Vitest files / 909 tests and production
  build passed.
- Backend `go vet ./...` and `go test ./...` passed.
- `verify-agent-legacy-cutover.sh` and its PostgreSQL 17 drill passed, including
  dry-run, real dump fingerprint, rejected count/fingerprint, exact JSONB key
  deletion, unrelated-row equivalence, restart and expected-zero replay.
- Product/Shadow source and PostgreSQL 17 gates, Phase 0 and
  `verify-standalone.sh --full` passed; full RAG result was 1906 passed / 7
  skipped.
- Quality/change gates passed. Security scan found no issue in changed frontend
  storage or scripts; two unchanged Go test fixture strings were classified as
  scanner false positives rather than credentials.
