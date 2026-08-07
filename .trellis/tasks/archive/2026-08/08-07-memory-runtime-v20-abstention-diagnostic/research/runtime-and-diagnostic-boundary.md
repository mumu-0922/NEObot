# Runtime recovery and v20 diagnostic boundary

## Runtime recovery evidence

- Live `backend` and `memory-worker` stopped after Docker Desktop lost the old
  WSL bind-mount handle. Their source keyring remains a regular mode-`0600`
  file; PostgreSQL remains healthy.
- The stopped containers name `mm-chat/backend:memory-recall-safe-5d421e3b`,
  but their historical image ID is no longer present in the Docker image
  store.
- The configured tag and the separately retained pre-v18-canary tag both
  resolve to image `sha256:501a8568...`.
- Without starting either image, the API and Memory Worker binaries were copied
  from the stopped containers and a temporary container made from the retained
  image. Both byte comparisons passed:
  - API SHA-256: `14cb0cbbb18ec31c44882c51826ab5995bc9c3e0d6629b79b619330243f3e450`
  - Worker SHA-256: `86fe98d68c4bb4811f918af7c209d64daf2b4f85b27edc2f9e5ba46bd01698e1`
- The temporary comparison container and copied binaries were destroyed. No
  Provider request or credential read was performed.

## Recommended recovery

Use the already configured retained tag only after recording database/container
state and rendering Compose. Recreate exactly `backend` and `memory-worker`
with `--no-build --no-deps --force-recreate`; do not pull, migrate, or restart
PostgreSQL. Require healthy services, byte-identical binaries, unchanged flags,
unchanged migration/relation counts, and unchanged unrelated container IDs.

## Recovery result

- An isolated PostgreSQL 17 rehearsal passed from a clean baseline using the
  retained image: build schema/roles/ACLs through `069`, truncate all public
  data, restore the exact v66 archive with triggers disabled, prove all 214
  foreign keys have no orphan, then apply only `067`–`069`.
- The identical live recovery completed in a new database. The prior empty
  `neo_chat` database remains retained as a connection-disabled rollback; it
  was never overwritten.
- Final live authority is migration `069` with 103 public tables, one user, one
  `user_memories` row, and ten `provider_configs` rows. Function ownership,
  capability ACLs, restricted roles, and forbidden memberships passed.
- Backend readiness exposed a separate Docker-restart drift: the MinIO bucket
  remained present, but the configured application Access Key no longer
  existed in MinIO IAM. Re-running the existing byte/config-matched one-shot
  `minio-init` container restored the user and policy without recreating that
  container or restarting MinIO; an independent bucket-access proof passed.
- The backend and Memory Worker are healthy on retained image ID
  `sha256:501a8568...`. PostgreSQL and every unrelated container ID are
  unchanged; both Memory flags remain false and the canary allowlist is empty.
- Recovery evidence is mode-`0600` under the ignored runtime directory
  `mm-chat/backup/postgres/memory-recovery-20260807T063402Z/`. No Provider
  request was made during recovery.

## Development diagnostic selection

The protected corpus was inspected only for Development split membership and
slice labels; no query or Memory plaintext was emitted.

```text
stable_fact Development cases = 30
temporal_correction Development cases = 30
intersection = 3
union = 57
```

Reuse the established three-repetition diagnostic design for exactly 171
executions. Keep schema-v20 prompt v2, BGE, decoder, retries, costs, and final
intersection unchanged. Retain only opaque stage membership and aggregate
classification. This is diagnostic evidence only and cannot occupy or unlock
the reserved schema-v21 Validation identity.

## Diagnostic implementation and Fake result

- The fresh identity is
  `neo-chat.memory-regression-v20-abstention-diagnostic.v1`; its profile,
  reader, run manifest, execution sequence, policy, and cost document are also
  independently versioned and do not consume schema-v21.
- Selection proved `stable_fact=30`, `temporal_correction=30`, intersection
  `3`, union `57`; three repetition-major passes produce exactly 171
  executions and at most 513 Luna attempts.
- The lane reuses schema-v20 prompt v2, fixed BGE tuple, buffered decoder,
  two-retry controller, cooldown, negative guard, and final intersection. Only
  opaque case identity, slice membership, stage counts, and normalized root
  cause are retained.
- Fake run `memory-regression-20260807t070853z-61c3884c` completed 171/171 with
  `classification=not_reproduced`, 171 Judge attempts, all root causes `none`,
  zero network, two mode-`0600` artifacts, and zero scoped container/network/
  volume residue.

## Sole live diagnostic result

- The only authorized live run
  `memory-regression-20260807t071130z-90224759` completed all 171 executions
  and retained the validated report plus manifest. Its classification is
  `stochastic`, not systematic.
- Luna did not select the expected current fact in six executions across four
  opaque cases: two cases failed once in three repetitions and two failed
  twice. No case failed all three repetitions. Four failures belonged only to
  `stable_fact`; two belonged only to `temporal_correction`; none was in the
  three-case slice intersection.
- Candidate, BGE rerank, final rank/budget, and terminal-failure root causes
  were all zero. Eleven typed `PROVIDER_TRANSPORT_FAILED` Judge attempts each
  recovered through retry, so 171 logical Judge decisions reconciled to 182
  attempts and 11 retries with zero terminal cases.
- Provider authority reconciled at `182/513` Judge requests,
  `320429/1000000` input-token upper bound, and `23296/65664` output-token
  upper bound. The 170 serial cooldowns reconciled to 170,000 configured and
  170,005 elapsed milliseconds.
- The report and manifest are mode `0600`; their SHA-256 values are
  `7c17325844e0eb5d4874386437f471fd54f6f061318c538f4e1d30026823936a`
  and `49b24b9f0cb914abcf6af7a15c659437c165ebc18c28c256ae4fca97f0f47e97`.
  The configuration, cost-basis, and case-order hashes are respectively
  `727d6e03776feee01518c8f6882981a66e83f2a9dd445dff3ac74fb9b39760c5`,
  `2cb43f78854f22c1c0144cc3eefe23e26fcf8e109699088a6751985075433014`,
  and `cbbec1525d0ff1a1bc7d01e7d5d1c43eab68a78d9d7f575a850666724e10e0f7`.
- Cleanup left zero scoped containers, networks, volumes, or credential
  directories. The live backend, Memory Worker, and PostgreSQL remain healthy;
  migration remains `069`, row counts remain `1/1/10` for users,
  `user_memories`, and `provider_configs`, both Memory flags remain false, and
  the canary allowlist remains empty.
- This evidence diagnoses stochastic Luna selection at the unchanged schema-v20
  prompt boundary. It selects no policy, grants no release/promotion/rerun, and
  does not construct or authorize schema-v21 Validation.

## Quality verification

- Focused race passed for `internal/usermemory`, `internal/memorycapture`, and
  `cmd/memory-regression-capture`.
- Full backend `go test ./...` and `go vet ./...` passed.
- Memory regression topology/lifecycle, the new Vault lifecycle across all
  exit/signal paths, both single-server Compose renders, and the isolated
  memory-regression Compose render passed.
- `verify-standalone.sh --full` passed, including frontend formatting/lint/
  typecheck, 964 tests and production build; all backend tests/vet; RAG Ruff,
  mypy, 1,906 tests with seven integration skips; every Memory wrapper
  lifecycle; and standalone copy checks.
- Change, quality, and security scanners passed with zero errors and zero
  Critical/High findings. Existing file-length warnings remain non-blocking;
  the changed-diff credential/private-key scan found no secret.
