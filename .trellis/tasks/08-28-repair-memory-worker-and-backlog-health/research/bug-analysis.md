## Bug Analysis: orphan Activity blocked the Memory claim loop

### 1. Root Cause Category

- **Category**: B/E — Cross-layer contract plus implicit lifetime assumption.
- **Specific Cause**: `memory_jobs` intentionally outlives Message deletion,
  while `message_memory_activities` requires a live owner-matching assistant.
  The dead-letter trigger assumed both lifetimes were identical. Its failed
  projection rolled back the authoritative lease terminalization.

### 2. Why Fixes Failed

1. **Image alignment**: Backend/Worker drift violated release policy, but both
   images exercised the same failing database trigger. Container health proved
   heartbeat/readiness only, not one successful queue iteration.
2. **Log inspection alone**: privacy-safe Worker logging intentionally omitted
   raw database errors, so the repeated warning identified the loop but not the
   failing lane. A transaction-scoped runtime-role function probe was needed.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Keep the job/error authoritative; make Activity conditional on a live owner row without weakening its FK. | DONE |
| P0 | Test coverage | PostgreSQL regression covers live and deleted assistants plus `109` down/re-up. | DONE |
| P1 | Documentation | Record differing durable/projection lifetimes and Claim transaction probing. | DONE |
| P1 | Runtime verification | Require queue movement and absence of iteration errors in addition to container health. | DONE |

### 4. Systematic Expansion

- **Similar issues**: Review/Usage/other audit projections triggered from
  durable objects can repeat this failure if they outlive their display owner.
- **Design improvement**: A derived link must never abort an authoritative
  lifecycle transition merely because the target was legitimately deleted.
- **Process improvement**: When a private worker logs only a bounded category,
  reproduce the exact capability inside a rollback-only transaction under the
  runtime role before changing queue state.

### 5. Knowledge Capture

- [x] Updated the Memory Activity executable contract.
- [x] Updated the cross-layer checklist.
- [x] Added migration and PostgreSQL 17 regression coverage.
- [x] Preserved rollback images, environment, database dump, and runtime
      identity evidence.
