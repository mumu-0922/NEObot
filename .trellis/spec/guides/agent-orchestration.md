# Agent Orchestration Guide

> **Purpose**: Use Sub-agents only when parallelism or independent review
> improves delivery without weakening correctness, context, or ownership.

## Decision Gate

Start Sub-agents when all applicable conditions hold:

- The work has at least two concrete, bounded streams that can progress
  independently.
- Streams are read-only or have disjoint file ownership; one file has only one
  writer at a time.
- Each Agent can receive enough context and an explicit deliverable.
- The Lead has a clear integration and verification plan.
- Coordination cost is lower than the expected time or quality benefit.

Do not start Sub-agents when the task is trivial, inherently sequential, still
depends on an unsettled shared contract, requires concurrent writes to the same
files, or would add more merge/review risk than value. Fall back to one Agent
whenever parallel execution begins to reduce quality.

## Role Allocation

- **Scout**: Read-only discovery, contract tracing, risk analysis, or test-plan
  research that can run beside the main implementation.
- **Worker**: Implementation with an explicit, disjoint file set and stable
  upstream contract.
- **Review Agent**: Independent review after a coherent implementation exists.
  Start one when the change is security-sensitive, cross-layer, large, hard to
  reason about, or likely to benefit from a fresh adversarial pass.
- **Lead**: Owns shared contracts, integration, conflict resolution, final
  verification, documentation, staging, and commits.

Do not start a Review Agent for trivial/docs-only changes, when automated checks
fully cover the risk, or when no stable diff exists to review. A requested
review remains mandatory unless it would duplicate an already-running review
with no additional scope.

## Execution Rules

1. State each Agent's scope, read/write ownership, expected output, and
   prohibition on unrelated edits.
2. Parallelize independent work; serialize producer/consumer or shared-contract
   work.
3. Never let multiple Agents edit the same file concurrently.
4. Treat implicit tool outputs as shared writes too. Give concurrent coverage,
   cache, report, build, and temp artifacts unique per-Agent paths (for example,
   `COVERAGE_FILE=.coverage.<agent>`), or serialize those commands; clean the
   artifacts after verification.
5. Keep commits and final integration under the Lead.
6. Verify every Agent result against live code and tests; do not merge reports
   by trust alone.
7. Stop spawning, reduce concurrency, or return to serial execution when
   context drift, conflicts, repeated failures, or quality regression appears.

## Quick Decision

```text
Independent bounded streams + disjoint ownership + net benefit?
  yes -> allocate only the needed Scout/Worker Agents
  no  -> execute serially

Stable high-risk or complex diff benefits from fresh review?
  yes -> start one focused Review Agent
  no  -> rely on Lead review and automated gates
```
