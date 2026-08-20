# Bug Analysis: Applied migration 096 was rewritten

## 1. Root Cause Category

- **Category**: C - Change Propagation Failure, with a D - Test Coverage Gap.
- **Specific Cause**: commit `209fd556` fixed two PostgreSQL function defects by
  editing the already-applied `096_chat_agent_event_log.up.sql`. Production kept
  the correct original checksum while later source embedded different bytes, so
  the runner rejected every later migration before changing schema.

## 2. Why the Earlier Fix Failed

1. The function-body correction fixed runtime behavior but treated migration
   source like ordinary application source rather than an immutable ledger
   artifact.
2. Schema tests asserted the corrected SQL fragments but did not pin the
   production-applied checksum, so the rewrite looked desirable in source-only
   verification.
3. Historical PostgreSQL drills assumed `097` remained the repository head and
   did not account for the irreversible `098` tail, leaving the rollback suite
   vulnerable to later head growth.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Restore exact applied `096` bytes and carry the repair in forward-only `099` | DONE |
| P0 | Test coverage | Pin the production `096` checksum and exercise fresh plus retained-ledger upgrade | DONE |
| P0 | Operations | Never edit `schema_migrations.checksum`; fail closed on drift | DONE |
| P1 | Test infrastructure | Defer later irreversible tail rows only inside disposable historical drills, then replay the real tail | DONE |
| P1 | Documentation | Add applied-migration immutability to Backend and cross-layer specs | DONE |

## 4. Systematic Expansion

- **Similar Issues**: every committed migration direction, including comments,
  whitespace, line endings, and terminal blank lines, can drift after apply.
- **Design Improvement**: live checksums for known production migrations should
  be pinned beside schema-contract tests; behavior changes use new migration
  numbers.
- **Process Improvement**: any schema-head change must search and update current
  head assertions plus older tail-peel drills in the same change.

## 5. Knowledge Capture

- [x] Updated `.trellis/spec/backend/agent-runtime.md` and Backend index.
- [x] Updated `.trellis/spec/guides/cross-layer-thinking-guide.md` and guide index.
- [x] Updated operations, migration, persistence, and deployment contracts.
- [x] Added checksum/schema tests and PostgreSQL 17 fresh/live/down-up proofs.
