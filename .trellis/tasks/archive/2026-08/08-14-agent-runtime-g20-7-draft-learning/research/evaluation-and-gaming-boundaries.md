# Draft evaluation and gaming boundaries

## Comparable patterns

- Secure build pipelines split static analysis, isolated execution and behavioral
  evaluation into independently identified suites.
- Evaluation systems bind every result to artifact and suite fingerprints; a
  pass for different bytes or a changed test set is not reusable.
- Safety review treats package text, tests and evaluator output as untrusted
  evidence, never as authorization.

## Conventions that apply here

- Exactly three checks are required: `static`, `isolation` and `evaluation`.
  Each receipt binds Draft/package/test fingerprints, suite fingerprint, outcome
  and a content-free evidence fingerprint.
- Check work uses owner/generation/expiry claims. A stale or expired worker cannot
  publish a pass. Infrastructure failure is bounded and never becomes a policy
  pass.
- Static policy rejects high-confidence prompt override, secret material, source
  laundering and evaluation-gaming patterns. Isolation/evaluation are injected
  control-plane adapters; there is no in-process or production executor in G20.7.
- Repeating failed policy checks is forbidden. A corrected proposal creates a
  new immutable Draft; this prevents trial-until-pass evaluation gaming.
- Automated checks may set only `reviewable` or `check_failed`; they cannot create
  a Skill candidate, make a package installable or call Promote.

## Repository mapping

- `internal/agentlearning` owns validation, diff construction, static policy and
  checker orchestration.
- PostgreSQL owns claim fencing, result immutability, state transitions and
  current Kill Switch denial.
- Tests use deterministic fake isolation/evaluation adapters. Passing those fakes
  is source/control evidence only and does not satisfy exact-host isolation.

