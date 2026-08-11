# Live schema-v21 Validation approval preflight failure

## Result

The fresh one-shot schema-v21 Validation invocation started and finished at
`2026-08-08T10:20:56Z` with status `2`. The Vault export completed, but the
generic runner rejected the forwarded approval before constructing either
Provider or creating the isolated regression runtime. There were zero BGE or
Luna requests and no report or manifest.

Both exported credential copies and the export container were destroyed. No
scoped container, network, volume, or credential directory remains. Private
aggregate-only evidence is retained under
`/var/tmp/neo-chat-abstention-confirmation-validation-preflight-20260808T102056Z`.

```text
raw cost SHA-256: cd60276abf34cdd2629cc68d416828154692990b93fbb5e06690733e4c4c442d
stdout SHA-256:   ccc4bacdcd64fe9c0958a0508975a07b83409a3881a1f09716e8ec892387f177
stderr SHA-256:   58236b2a6c2c5de5b3ba2163c7868a752a1cc544fa69482e80c160571e944719
```

## Bug analysis

### 1. Root cause category

- **Category**: B/C/D — cross-layer contract, change-propagation failure, and
  test-coverage gap.
- **Specific cause**: The wrapper and its isolated mock test accepted and
  forwarded
  `I_UNDERSTAND_THIS_USES_REAL_MEMORY_ABSTENTION_CONFIRMATION_VALIDATION_QUOTA`,
  while the generic runner and Go live gate required the frozen-Validation
  literal with the additional `FROZEN` segment.

### 2. Why the earlier fix missed it

1. The prior credential-mount repair aligned tuple validation, file copy, and
   mount predicates but did not audit the approval literal as another
   cross-layer mode surface.
2. The Vault test's fake runner duplicated the wrapper's incorrect literal, so
   both sides agreed with each other while disagreeing with the real runner.
3. Generic-runner tests exercised the correct literal directly and therefore
   did not traverse the Vault wrapper boundary.

### 3. Prevention mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Runtime | Keep the generic runner and Go live gate fail-closed before Provider construction | Done |
| P0 | Test | Make the Vault fake runner require the frozen literal | Done |
| P0 | Test | Assert the same literal exists in the wrapper, generic runner, and Go live gate | Done |
| P1 | Documentation | Add approval identity to the mode-propagation checklist | Done |

### 4. Systematic expansion

- **Similar issue**: Every new live mode crosses wrapper parsing, wrapper exact
  approval, generic parsing, generic mode approval, environment forwarding,
  Go live gate, and mock-runner expectations.
- **Design improvement**: Treat the approval literal as part of the same
  versioned mode tuple as credential source/target and capture identity.
- **Process improvement**: A live-shaped wrapper test must compare against the
  production runner/gate contract, not only a self-authored mock contract.

### 5. Gate state

The invocation consumed its exact one-shot authority and is not rerun. Live
schema-v21 Validation remains incomplete. Product services are healthy,
Memory flags remain false, the canary is unset, and v1 remains authoritative.
