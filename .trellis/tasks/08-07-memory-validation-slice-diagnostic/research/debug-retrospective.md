# Debug retrospective: schema-v20 prompt/token authority drift

## 1. Root Cause Category

- **Category**: C — Change Propagation Failure, crossing a B — Cross-Layer
  Contract boundary.
- **Specific cause**: schema-v20 selected Luna prompt v2 in the buffered
  Provider adapter, while the capture controller still derived its
  pre-authorized input-token upper bound from prompt v1. The Provider request
  and cost ledger therefore described different bytes.

## 2. Why the first Fake failed

1. The semantic prompt change was correctly isolated in the adapter, but the
   token estimator was treated as bookkeeping rather than another consumer of
   the same prompt authority.
2. Unit tests covered adapter prompt provenance and capture reconciliation
   independently; only the full Fake lifecycle exercised both boundaries with
   the same case plan.

## 3. Prevention mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Inject one versioned prompt builder into both the adapter and capture controller. | Done |
| P0 | Test coverage | Compare legacy/v2 request shape and prove Fake token reconciliation end to end. | Done |
| P1 | Documentation | Forbid duplicated prompt assumptions in the Memory benchmark spec. | Done |

## 4. Systematic expansion

- **Similar issues**: request signing, egress hashing, cost estimation, and
  retry accounting can drift whenever they reconstruct a versioned Provider
  payload independently.
- **Design improvement**: treat payload builders as authority-bearing
  dependencies, not formatting helpers.
- **Process improvement**: every Provider payload version change must enumerate
  request construction, provenance, token/cost accounting, fixtures, and
  report validation as one propagation set.

## 5. Knowledge capture

- [x] Updated `.trellis/spec/backend/memory-v2-benchmark.md` with the shared
      builder contract, failure behavior, test requirement, and wrong/correct
      example.
- [x] Added regression tests and completed the network-free 300-case Fake
      lifecycle.
- [x] No template mirror exists in this repository, so no template sync is
      applicable.
