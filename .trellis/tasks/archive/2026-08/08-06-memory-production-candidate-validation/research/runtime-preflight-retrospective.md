# Runtime preflight retrospective

## 1. Root cause category

- **Category E — implicit assumption**: the wrapper assumed every installed
  Compose `run` implementation accepted `--no-build`. Docker Compose v5.3.1
  removed that negative flag even though `run` still builds only when the
  positive `--build` flag is supplied.
- **Category B/D — cross-layer contract and test gap**: source and mocked
  wrapper tests assumed the configured admin image contained the new
  `memory-validation-credentials-export` command. The live Compose project was
  intentionally pinned to an older binary, so current source capability did
  not prove runtime image capability.

Both failures occurred before credentials or Provider work. They were retained
as zero-request preflight evidence and did not consume the one complete live
Validation authority.

## 2. Why the first fixes were incomplete

1. The original mock asserted literal `--no-build --pull never` but never
   executed `docker compose run --help` from the installed CLI. It proved the
   intended argument list, not compatibility with runtime truth.
2. Capability-detecting `--no-build` fixed the CLI surface and reached the next
   boundary. It did not fix the independent assumption that the active old
   admin image implemented a command added in newer source.
3. Building an explicitly named candidate image was necessary, but its first
   capability assertion used a `pipefail` pipeline where expected admin usage
   status `1` overrode a successful `grep`. Capturing usage output before
   matching removed that shell-status ambiguity.

## 3. Prevention mechanisms

| Priority | Mechanism | Action | Status |
| --- | --- | --- | --- |
| P0 | Runtime capability | Inspect `compose run --help` before credentials; always pass `--pull never`, never pass positive `--build`, and add `--no-build` only when supported. | Done |
| P0 | Immutable image boundary | Build/tag the reviewed candidate explicitly and render the helper service against that exact tag; do not move the live mutable tag or recreate a service. | Done |
| P0 | Test coverage | Exercise both Compose capability branches and verify credential cleanup on success, metric failure, ordinary failure, and signals. | Done |
| P1 | Runtime smoke | Prove the pinned admin binary's usage contains the exact helper command before the first credential export. | Done |
| P1 | Rollback evidence | Retain a separately named image whose four backend binaries hash exactly to the running container before any possible recreation. | Done |

## 4. Systematic expansion

- Historical consumed Vault wrappers still contain literal
  `run --no-build`; they may fail on Compose v5. They must not be silently
  edited or rerun because their evidence identities are immutable. New
  successor wrappers must use capability detection.
- Source tests cannot establish a live binary's command set. Any one-off
  operator command must probe the exact pinned image that will execute it.
- A running container's `.Image` value may be a BuildKit config digest that is
  not directly taggable. A retained local image is acceptable only after its
  relevant binaries are compared byte-for-byte with the running container.
- Shell tests that intentionally inspect commands returning non-zero usage
  status must not combine the producer and matcher under `pipefail` without
  handling the producer's expected status explicitly.

## 5. Knowledge captured

- `.trellis/spec/operations/runtime-recreate-image-pinning.md` now defines the
  Compose capability branch and pinned helper-image rule.
- `.trellis/spec/backend/memory-v2-hybrid-shadow.md` and
  `.trellis/spec/backend/memory-v2-benchmark.md` define the schema-v18
  pre-provider, cleanup, and failed-slice no-rollout boundaries.
- The wrapper lifecycle test covers Compose implementations with and without
  `run --no-build`.
