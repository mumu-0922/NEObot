# Short overlap rejection root cause

## Observed production path

1. The uploaded DOCX passed source SHA-256 verification.
2. The sandbox parser returned a valid `docx` Native artifact with 63 nodes.
3. Native structure extraction produced 60 projected text units.
4. Structure planning produced 4 Parents and 19 Children.
5. Several Child transitions reused whole previous-child atoms totaling fewer
   than the mapper minimum: 58, 56, 38, and 51 tokens.
6. `build_native_structure_artifacts(...)` rejected the first such transition
   in `_chunk_fragments(...)` as `NATIVE_STRUCTURE_ARTIFACT_INVALID`.

## Contract mismatch

- The planner targets 64 tokens and caps overlap at 100 tokens, but it has no
  minimum gate.
- Native and MinerU artifact mappers independently hard-code a 60-token minimum.
- Whole-atom selection can legitimately stop below 60 when prepending the next
  earlier atom would exceed 100.

## Selected correction

Keep exact prior-child fragment reuse. If the bounded whole-atom suffix does not
reach 60 tokens, represent the transition as having no overlap. Do not relax the
mapper validator and do not synthesize a partial fragment that was absent from
the previous Child.

## Runtime scope

The `test` collection has three other current documents and all three are
active. The newly uploaded DOCX is the only current document affected by this
error. Historical G7.8 smoke failures are unrelated operational fixtures and
remain outside this repair.

## Bug Analysis: short exact overlap rejected a valid document

### 1. Root Cause Category

- **Category:** B/D — Cross-Layer Contract plus Test Coverage Gap.
- **Specific Cause:** The planner had no minimum gate, while both artifact
  mappers independently required 60 tokens. Its loop also retained the token
  count of a rejected `>100` candidate instead of the fragments actually
  selected.

### 2. Why Earlier Repair Did Not Close the Upload

1. The first repair correctly admitted the DOCX pagination marker and exposed
   the next independent pipeline failure.
2. Existing parser tests stopped at Native artifact creation; existing long
   structure fixtures generated only overlap values already above 60.
3. No test carried a 1–59-token whole-atom suffix through the planner and
   artifact mapper together.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Share `OVERLAP_MIN_TOKENS` across planner and both mappers | DONE |
| P0 | Test | Add below-minimum planner and Native artifact regressions | DONE |
| P1 | Spec | Record the zero-overlap fallback and mapper fail-closed rule | DONE |
| P1 | Review | Add producer/validator bound checks to the cross-layer guide | DONE |

### 4. Systematic Expansion

- **Similar Issues:** MinerU used the same duplicated 60-token literal and is
  repaired by the shared constant even though this incident used DOCX.
- **Design Improvement:** Producer output must be a subset of consumer-admitted
  values; quality fallbacks are explicit absence rather than invalid data.
- **Process Improvement:** Parser compatibility fixtures must cross the full
  parse → plan → artifact boundary, not stop at parser output.

### 5. Knowledge Capture

- [x] Updated the executable RAG backend spec.
- [x] Updated the cross-layer thinking guide.
- [x] Updated the product structure-chunking contract.
- [x] Added regression tests and a real-source replay proof.
