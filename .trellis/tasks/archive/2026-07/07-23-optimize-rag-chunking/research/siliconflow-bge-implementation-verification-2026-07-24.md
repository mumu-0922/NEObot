# SiliconFlow Pro BGE implementation verification — 2026-07-24

## Scope

This verification covers the code and migration boundary for the isolated
SiliconFlow Candidate Search Profile:

```text
Embedding: Pro/BAAI/bge-m3
Reranker:  Pro/BAAI/bge-reranker-v2-m3
Profile:   siliconflow_bge_m3_v1
```

It did not call the real provider, read an API Key, allocate or activate a
Candidate, execute Holdout, move the retrieval pointer, commit, or archive the
task.

## Cross-layer result

- Migration `049` admits BGE as a separate vector space while preserving the
  migration-048 legacy Jina readers.
- Go selects Query Embedding from the Active Generation/Search Profile and
  passage Embedding from immutable Generation authority.
- A Profile race discards the stale vector and retries the complete bind once.
- Provider failure can use only same-Generation/Profile fenced BM25.
- Rerank uses the Profile carried by the retrieved evidence rather than a new
  Active lookup.
- `RAG:SILICONFLOW` is a distinct Vault record; Python never receives a reusable
  credential.
- Frontend admin/status types, settings, health, and English/Chinese/Japanese
  copy enumerate `siliconflow`.

## Disposable PostgreSQL 17 drill

Image:

```text
mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5
```

Observed sequence:

```text
fresh up:                head 49 / 49 rows
fresh BGE profiles:      0
clean down:              head 48 / 48 rows
generation resolver:     absent after down
re-up:                   head 49 / 49 rows
fake BGE Index Profile:  inserted
down result:              RAG_SILICONFLOW_ROLLBACK_REQUIRES_BGE_PURGE
atomic postcondition:     head 49 / 49 rows, BGE profile count 1
migration checksum:       64 hexadecimal characters
```

The isolated PostgreSQL integration test
`TestPostgresFetchQueryEvidenceCandidatesReturnsBoundedReferences` also passed
against a blank disposable test database. The test database must not be
pre-migrated: the test runner creates a strict per-test schema and installs the
relocatable retrieval extensions there.

## Quality gates

```text
RAG Ruff:                 passed
RAG mypy:                 passed (68 source files)
RAG pytest:               1778 passed / 7 skipped
Go gofmt/diff check:      passed
Go vet ./...:             passed
Go test ./...:            passed
Frontend Prettier:        passed (changed files)
Frontend ESLint:          passed
Frontend TypeScript:      passed
Frontend Vitest:          920 passed
Security scanners:        5 target areas passed, zero findings
Exact tuple consistency:  Go / SQL / Python / UI / spec passed
Added secret-like lines:  0
```

## Remaining live boundary

An administrator must configure and enable `RAG:SILICONFLOW`, then Development
and Validation must capture real provider correctness, latency, and quota
behavior before a new Candidate can become promotion-eligible. Activation and
the one-shot Holdout remain explicitly forbidden at this stage.

## Local single-server deployment proof

The running local stack initially served stale artifacts:

```text
database migration head:       47
frontend SiliconFlow bundle:   absent
backend SiliconFlow route:     absent
```

The first live `049` attempt correctly rolled back after exposing a stored-data
branch not present in the fresh drill. Four consent-backfill statements used
separate `clock_timestamp()` calls, so `decided_at` could precede the later
`created_at` value and violate `processing_consents_decided_after_created`.
The fix uses one transaction-stable `CURRENT_TIMESTAMP` for `decided_at`,
`created_at`, and `updated_at`, with a static four-tuple regression test.

After rebuilding Backend, Frontend, and RAG Worker, applying migrations, and
recreating the services:

```text
database migration head:       49
backend health:                 healthy
frontend health:                healthy
RAG Worker health:              healthy
frontend SiliconFlow bundle:   present
backend SiliconFlow route:      present
Go vet/test after fix:          passed
```

No provider Key was read and no real SiliconFlow request, Candidate allocation,
Activation, or Holdout occurred during deployment.

## Configured-provider Development smoke

After the administrator configured `RAG:SILICONFLOW`, the atomic UI operation
reported two passed checks, covering the fixed Embedding and Rerank sentinels.
The private Go Provider Gateway passage-Embedding path was then exercised with
a non-private Development sentinel and returned:

```text
model:          Pro/BAAI/bge-m3
dimensions:     1024
vector count:   1
components:     finite
norm:           non-zero
```

The reusable credential remained inside the Go Vault boundary and was neither
read nor printed. The Worker profile was moved from
`mineru_jina_postgres_v1` to
`mineru_jina_siliconflow_postgres_v2`; the recreated Worker is healthy and can
admit both the retained Jina Active vector space and the new BGE Candidate
space.

At that checkpoint, Candidate sequence `7` remained `verified/ready` on the
superseded pre-BGE Chunk Profile and occupied the singleton Candidate slot. It
was not abandoned automatically. The next section records the later, explicitly
approved audited abandonment; Active sequence `3` remained unchanged.

## BGE Candidate sequence 8

After explicit operator approval, Candidate sequence `7` was abandoned through
the exact manifest/head CAS gateway:

```text
candidate generation: 53cfdad8-4e69-4d9e-a4c0-d2fcaec29696
artifact manifest:    7d5507b73294d5bbcb95862f858d2f9dd9ea3cc3473d078604801244d3a1de9b
operator UUID:        1f53685f-b00d-4d8f-8d0d-a7f521b5246b
head revision:        4
result:               failed / OPERATOR_ABANDONED
```

The Worker was stopped before allocation, preserving the single-image fence.
The new immutable BGE Candidate was then allocated and processed by the same
pinned Worker image:

```text
candidate generation: 4e9e18ef-c259-440b-9976-b4632e50b419
candidate sequence:   8
index profile:        476479f9-a586-4092-b85e-cf6f575425d1
search profile:       23a7b73b-97b8-4b4c-8b66-e25ad7543ccb
chunk profile:        36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73
base profile:         802feafc35483924947c58b6c1e129359f24492a1e76d4db1106b16cda5c8803
search profile hash:  4b579f2b23d25af79299a1a27ecfaf9e7db3d9ef57180b14d5c41a4d08feff3c
build snapshot:       aff33178e3722d4ab5bb1ebe8a53a4267a5847e57e11f32b2ec0e64a0e82e4a5
```

All 110 jobs drained without failure or cancellation: 55 Parse and 55 Passage
Embedding jobs. Verification was executed twice with identical results:

```text
status/readiness:       verified / ready
documents:              55
blocks:                 846
parents:                147
children:               150
maximum Parent tokens:  685
maximum Child tokens:   397
Parents with headings:  51
Children with overlap:  3

Search rows:            150
BGE model rows:         150
non-null BGE vectors:   150
distinct Search Profile: 1

artifact manifest:
ae72c08e56989f7f831fdf42cedc2d7febb846f92481bd79088b6ac8819f562f

verification report:
bb7dfb151735ac26bd21e16eb5c21b496be02916b971ff1e010551274850536e
```

The exact Search tuple is
`siliconflow_bge_m3_v1 / Pro/BAAI/bge-m3 / 1024 /
Pro/BAAI/bge-reranker-v2-m3`. `promotionEligible=false`. Active remains Jina
sequence `3`, head revision `4`; no Activation or Holdout was executed.

## Development/Validation preflight execution

The first Candidate 8 smoke exposed a live SiliconFlow response detail not
present in the fixture: even with `return_documents=false`, every Rerank result
included `document:null`. The strict response decoder rejected that otherwise
valid response and exhausted all six retries as `rerank unavailable`.

The adapter now admits only an absent or JSON-null `document` field. It still
rejects an object, string, or any other returned document body, so the fixed
wire behavior does not broaden source disclosure. Unit coverage exercises both
the observed null response and the fail-closed body case.

After rebuilding the standalone capture binary, a one-case
Development/Validation smoke passed end to end:

```text
artifact:
promotion-preflight-v3-candidate8-smoke-2026-07-24.json

SHA-256:
d98f81c71f5d00574e16188352f4f53e31e7e3bda1366d9549f344d9a4b0be0f

capture version:      neo-chat.rag-promotion-capture.v3
Active cases:         1
Candidate cases:      1
Active quality:       1.0
Candidate quality:    1.0
Candidate latency:    850ms
promotionEligible:    false
Holdout:              not_executed
```

The first 400-case run at concurrency `4` failed closed on a transient Jina
Rerank outage and produced no partial artifact. A serial retry reached the last
`json_code` documents but hit the command's original 45-minute context
deadline. The output writer again remained atomic. The command deadline is now
120 minutes so a deliberately serial accuracy run can cross one bounded
provider cooldown without increasing concurrency.

An immediate serial restart then failed on the first Active Query Embedding.
Credential-free TLS probes confirmed that `api.jina.ai` was closing the TLS
handshake with an unexpected EOF from both the host and the isolated capture
network, while the SiliconFlow endpoint completed TLS normally. This is a live
Active-provider availability block, not a Candidate BGE failure. No full
Development/Validation report, Holdout execution, Activation, or Active pointer
change was claimed from the failed attempts.

After a quiet cooldown, a later serial run advanced for approximately 40
minutes to `rageval-code-zh-01-f03` before the old Active Jina Rerank path again
became unavailable. This showed that the original six attempts and 15.5-second
retry window cannot bridge the repeatable provider cooldown. Capture version
`v4` therefore records a bounded `26`-attempt policy with a 500ms initial delay
and 60-second maximum delay, providing about 19 minutes of recovery time inside
the 120-minute command deadline. Cancellation and the final deadline still
fail closed, and atomic output still prevents a partial report from being
mistaken for complete evidence.

The direct Jina route remained unavailable during the last execution window;
the final v4 run was stopped before wasting the complete retry window. Current
database status is still Active sequence `3`, Candidate sequence `8`
`verified/ready`, head revision `4`, corpus projection revision `298`, and the
same Candidate manifest. The v3 one-case smoke is the latest complete live
preflight artifact; the full 400-case report remains blocked on the old Active
provider rather than on SiliconFlow.
