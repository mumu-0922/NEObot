# MM Chat RAG Worker — Phase 15.2B

Python 3.13 durable-consumer mechanics for MM Chat. This package is deliberately
**dark-run by default**: it validates PostgreSQL functions and the singleton
worker lock, then serves health and metrics without claiming outbox events or
processing jobs.

Phase B does not parse files, call embedding providers, read object storage,
build search projections, or purge data. `DISPATCH_REGISTRY` and
`JOB_HANDLER_REGISTRY` are intentionally empty until Phase 15.2C promotion.

Phase 15.2C C0 adds **test-only** redacted Provider Contract intake under
`tests/fixtures/provider_contracts/`. The checked-in MinerU/Jina fixtures remain
blocked drafts; they do not enable provider calls or production handlers.

Phase 15.2C C1.1 adds installable, versioned Offline Parser schemas under
`src/mm_chat_rag/contracts/` plus test-only parser corpus and RFC 8785
cross-runtime gates. This freezes artifact shapes and hash inputs only; it does
not implement a parser, create derived output, or enable a runtime registry.

Phase 15.2C C1.2 adds the runtime-inert `mm_chat_rag.offline_parser` harness:
Magic/Container-first routing, exact MMCP v1 framing over a private Unix socket,
an exec-isolated child with pre-source process-group/seccomp handshakes, and a
marker/lock/dir-FD owned test-output root. The Compose `parser-c1` profile runs
the sidecar as PID 1 under UID `10002`, with no network, credentials, database,
Redis, MinIO, Provider access, or production handler registration. A routed
format still returns `FORMAT_UNSUPPORTED`; C1.2 never forges a success
candidate.

Phase 15.2C C1.3A adds deterministic TXT, Markdown, and HTML Native Parsers
behind that same Child/Seccomp boundary. Their closed
`parser-native-artifact.v1` output is Child-internal only: the Supervisor
validates JCS, length, hash, limits, format, and exact Source binding, but MMCP
success remains frozen to future `canonical-ir.v2`. The Sidecar therefore still
returns a zero-body, non-stageable `FORMAT_UNSUPPORTED`; production Registry,
Dispatch, Provider, Postgres, Redis, MinIO, and migrations `011/012` remain
closed. Text decoding is frozen as BOM -> UTF-8 -> GB18030; Markdown uses fixed
CommonMark + Table semantics, and HTML uses a hardened policy that blocks
active content and external fetches.

Phase 15.2C C1.3B upgrades the Child-internal contract to
`parser-native-artifact.v2` and activates fixed CSV, DOCX, PPTX, and XLSX
Native Parsers. Router and OOXML Parsers share one hardened
`ValidatedOpcPackage`: ZIP/OPC/XML admission is not duplicated, External
Relationships are metadata-only, formulas are never evaluated, and no Part or
URL is fetched. C1.3B adds no third-party dependency. MMCP still accepts only
future `canonical-ir.v2`, so Sidecar output remains zero-body
`FORMAT_UNSUPPORTED`; production Dispatch, Provider, Postgres, Redis, MinIO,
and migrations `011/012` stay closed.

The operator-only C0 Provider Capture Harness lives in
`tools/provider_capture.py`. It defaults to a redacted, zero-network dry-run and
is not a production console script. Real egress requires explicit `--execute`
plus process-environment credentials. A WSL operator may explicitly set
`PROVIDER_CAPTURE_PROXY_URL` to a literal private/loopback HTTP proxy; generic
proxy variables remain ignored. MinerU capture is intentionally staged at the
v4 local-upload Submit response: it does not PUT the signed URL or poll. A
separate `tools/provider_capture_mineru_lifecycle.py` CLI implements the full
bounded synthetic Allocate/Upload/Poll/Download chain with Evidence v2. Its
first two real Captures reached `done` but ended as `unknown_download`; the
second classified the proxy-routed Download failure as `connect_error`. An
Owner-authorized all-direct Capture then reached `download_failed`, proving only
that transport was crossed before a redacted response/archive gate failed. A
follow-up diagnostic narrowed that Gate to `archive_invalid`; future Evidence
may add only a closed `archiveFailureClass` without Entry names or content. It
remains outside Runtime, no automatic retry is allowed, and no Fixture was
promoted.

The root cause was a Provider-profile naming mismatch: Cloud v4 calls the Middle
artifact `layout.json`, while local/open-source output uses `middle.json` or
`*_middle.json`. The Harness maps both to one semantic Role without recording
the Entry name. A repaired all-direct Capture completed with all four Role
Presence booleans true; this remains Acquisition Evidence only.

## Safety boundaries

- PostgreSQL is authoritative. Runtime SQL calls frozen `SECURITY DEFINER`
  functions only, except PostgreSQL's built-in session advisory lock.
- The worker never runs DDL, schema migration, ORM, Alembic, or direct table DML.
- Redis is an optional, lossy wake hint (`payload == "1"`). One-second PostgreSQL
  polling and 30-second forced rescans continue when Redis is absent or flushed.
- Logs redact credentials, URLs, payloads, query/body/content, tokens, and object
  keys. Metrics have only fixed-cardinality labels.
- Replay uses a separate `RAG_REPLAY_DATABASE_URL` and is dry-run unless
  `--execute` is supplied with all CAS/audit inputs.

G11.9D.1 adds a pure structure-aware Parent/Child planner in
`mm_chat_rag.structure_chunking`. It accepts only validated structural text
units and returns UTF-8 byte-range plans. It performs no I/O. The planner is
now reachable only when a leased parse job resolves to the shared candidate
profile; Jina embedding and active-generation cutover remain separate gates.
The frozen contract and D.2/D.3 promotion gates are documented in
[`../docs/contracts/rag-structure-chunking.md`](../docs/contracts/rag-structure-chunking.md).

G11.9D.2.1 adds `mm_chat_rag.native_structure_artifacts` as the first consumer
of that planner. It maps validated Native headings, paragraphs, list items,
table rows, code, heading ancestry, and verified source positions into
schema-valid Canonical IR v2 / Chunk Manifest v2, including exact overlap and
Postgres projection DTO proof. The current production Native gateway still
uses the baseline profile: this module performs no persistence, provider call,
or generation mutation until MinerU parity and a new generation are staged.

G11.9D.2.2 adds `mm_chat_rag.mineru_structure_artifacts`. It consumes the
existing hash-bound MinerU mapping input and treats admitted
synthetic `middle_json.pages[].elements[]` or live-provider `pdf_info[]`
structure—not compatibility `full.md`—as authority for heading, text, table,
formula, page, and BBox projection. Unknown text-bearing kinds fail closed.

Native and MinerU structure manifests intentionally share one
`STRUCTURE_CHUNK_PROFILE_HASH`; one generation cannot admit provider-specific
chunk hashes.

G11.9D.2.3a adds the Postgres
`knowledge_begin_structure_generation_rebuild(...)` allocation boundary. It
creates only a non-active `building` generation, shared Index/Search Profiles,
and exact per-active-document staging materializations plus pending parse jobs.
It rejects incomplete, substituted, duplicate, or concurrent candidate sets
and cannot promote the generation.

G11.9D.2.3b adds the lease-fenced
`knowledge_resolve_parse_chunk_profile(...)` boundary. The worker preserves the
old text-baseline route for the baseline hash and selects the Native/MinerU
structure gateway only for the shared candidate hash. A disposable-clone live
proof projected one real MinerU PDF and two Native DOCX documents, retained PDF
page-BBox locators, and created exactly three pending passage-embedding jobs
without consuming Jina or changing the active generation. Unknown profile or
processor identities fail closed. Candidate generation verification, cutover,
and live citations remain later slices.

G11.9D.2.3c reuses the existing admitted Jina passage handler without adding a
second embedding path. A credential-backed disposable-clone run processed all
three candidate embedding jobs once, stored validated
`jina-embeddings-v4`/1024 vectors for the shared-profile Children, published
all three materializations and generation-scoped document heads, and proved
exact document coverage. That boundary deliberately left the candidate
`building`; G11.9D.3a below now freezes its manifest/counts, while atomic
cutover and citations remain later gates.

G11.9D.3a adds the database-only
`knowledge_verify_structure_generation(...)` boundary. It locks the corpus
head, derives exact current-document coverage and all Block/Parent/Child/vector/
locator counts from persisted evidence, verifies the latest job pairs and
generation-scoped heads, marks parser artifact sets verified, and freezes one
ordered deterministic manifest. Success changes only the candidate to
`verified` and its projection state to `ready`; deterministic replay must return
the same manifest/counts. It has no active-head or promotion path.

G11.9D.3b hardens `knowledge_promote_index_generation(...)` so it locks the
same corpus-head row as deletion and reruns the D.3a verifier in the promotion
transaction. A stale verified candidate therefore cannot cross a concurrent
delete. `knowledge_fail_structure_generation(...)` records the exact manifest
and failure code as an idempotent `failed/failed` rollback, leaving the active
generation unchanged and releasing the rebuild slot. Migration 032 exposes
only failure rollback to the Go runtime and explicitly revokes promotion;
D.3c retains successful cutover.

G11.9D.3c grants the D.3b-fenced promotion and adds
`knowledge_rollback_index_generation(...)`. Rollback is bound to the active
structure rebuild's exact `sourceGenerationId`, both manifests, the expected
head, current document/version/file/content coverage, and target
Parent/Child/ready-vector completeness. A real three-document run activated the
structure generation, produced an `answered`/`[K1]` citation bound to its Parent
and Child, then restored the old generation and produced a second `[K1]`
citation bound to that restored head.

## Local quality gates

No command loads repository `.env` files.

```bash
uv sync --locked
uv run ruff check .
uv run ruff format --check .
uv run mypy src tests/support
uv run pytest --cov=mm_chat_rag --cov-report=term-missing
uv run pip-audit --skip-editable
./tests/docker-smoke.sh
```

The Docker smoke builds the image, creates an isolated disposable PostgreSQL
fixture, starts `rag-worker` as UID `10001` with a read-only root filesystem,
waits for `/health`, and executes `rag-replay --help`. It does not read repository
`.env` files or connect to deployment services.

Integration tests skip only when their explicit DSN is absent:

```bash
RAG_TEST_DATABASE_URL='postgresql://...' uv run pytest -m integration
RAG_TEST_REDIS_URL='redis://...' uv run pytest -m integration
```

The PostgreSQL DSN must target a database migrated through `010` and a role with
only the Phase 15.2B worker grants.

Provider Contract validation and in-memory replay do not read `.env`, start a
listener, or make network calls:

```bash
uv run pytest -p no:cacheprovider tests/unit/test_provider_contracts.py
```

Offline Parser Contract, corpus, and JCS gates are also network-free:

```bash
uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_contracts.py \
  tests/unit/test_parser_artifact_schemas.py \
  tests/unit/test_parser_normalization_semantics.py \
  tests/unit/test_parser_projection_semantics.py \
  tests/unit/test_parser_lineage_semantics.py \
  tests/unit/test_parser_hash_dag_semantics.py \
  tests/unit/test_parser_stable_error_matrix.py \
  tests/unit/test_parser_corpus.py \
  tests/unit/test_parser_runtime_boundary.py \
  tests/unit/test_parser_package_artifacts.py \
  tests/unit/test_jcs_interop.py
uv run python -B -m tools.verify_jcs_interop --require-all
```

C1.2 router/protocol/sandbox/output-root plus C1.3B Native Parser gates and the
isolated Compose smoke:

```bash
uv run pytest -p no:cacheprovider \
  tests/unit/test_parser_router.py \
  tests/unit/test_parser_protocol.py \
  tests/unit/test_parser_sandbox.py \
  tests/unit/test_parser_output_root.py \
  tests/unit/test_parser_transport.py \
  tests/unit/test_parser_deployment_boundary.py \
  tests/unit/test_parser_native_model.py \
  tests/unit/test_parser_native_decoding.py \
  tests/unit/test_parser_native_text.py \
  tests/unit/test_parser_native_markdown.py \
  tests/unit/test_parser_native_html.py \
  tests/unit/test_parser_native_csv.py \
  tests/unit/test_parser_native_xml.py \
  tests/unit/test_parser_native_opc.py \
  tests/unit/test_parser_native_docx.py \
  tests/unit/test_parser_native_pptx.py \
  tests/unit/test_parser_native_xlsx.py \
  tests/unit/test_parser_native_dispatch.py \
  tests/unit/test_parser_native_internal_result.py \
  tests/unit/test_parser_native_sandbox.py

docker compose -f ../compose.single-server.yml --profile parser-c1 build \
  parser-sidecar
docker compose -f ../compose.single-server.yml --profile parser-c1 up \
  --abort-on-container-exit --exit-code-from parser-harness-smoke \
  parser-harness-smoke
```

The smoke sends only an ambiguous synthetic text request and expects the closed
`FORMAT_AMBIGUOUS` response. It does not mount Source files, produce a Native
Artifact, connect a production Worker, or leave the Parser Registry enabled.

The second command is the mandatory C1.1 interoperability gate. It fails when
Go 1.22 or Node 22 is absent or when Python, Go, and JavaScript disagree on any
checked-in byte/hash vector; it never downloads a dependency.

Wheel packaging is a separate two-step offline gate. The verifier consumes an
existing wheel and cannot build, install, or download anything itself:

```bash
uv build --offline --wheel --out-dir /tmp/mm-chat-rag-wheel
uv run python -B tools/verify_contract_wheel.py \
  /tmp/mm-chat-rag-wheel/mm_chat_rag-0.1.0-py3-none-any.whl
```

The Provider Capture Harness also makes no network call by default and creates
no evidence file in dry-run mode:

```bash
uv run python -B -m tools.provider_capture
uv run python -B -m tools.provider_capture_mineru_lifecycle
uv run pytest -p no:cacheprovider tests/unit/test_provider_capture.py
uv run pytest -p no:cacheprovider \
  tests/unit/test_provider_lifecycle_capture.py \
  tests/unit/test_provider_lifecycle_security.py
```

Authorized execution, exact budgets, evidence schema, review/freeze procedure,
and rollback are specified in
[`../docs/contracts/provider-capture-harness.md`](../docs/contracts/provider-capture-harness.md).
The example credential file contains empty values only. The harness does not
load it or any other dotenv file. `-B` is required by the CLI contract so a
dry-run creates neither evidence nor Python bytecode cache files. MinerU
`unknown_submission` writes its recovery evidence but returns exit code `3`.
Generated `provider-capture-*/` directories are ignored as a defense in depth;
reviewed Evidence must still be moved to a private Git-external store.
Provider Keys must be injected with a complete no-echo prompt or controlled
Secret mechanism. Do not implement partial visibility with repeated
`read -s -n1`: long clipboard pastes can leak into terminal scrollback.
When direct WSL egress is unavailable, copy the Owner-controlled proxy into the
dedicated variable for that one subshell, for example
`export PROVIDER_CAPTURE_PROXY_URL="$https_proxy"`; the harness validates a
literal RFC1918/loopback address and still uses `trust_env=false`.

`jsonschema`, `types-jsonschema`, `rfc8785`, and `httpx` are dev-only. Docker
uses `uv sync --no-dev`, and `.dockerignore` excludes `tests`, so fixtures and
Fake Provider support do not enter the runtime image. The Dockerfile copies only
`src/`; `tools/` is neither copied nor registered in `[project.scripts]`.
`markdown-it-py==4.2.0` remains the sole Native Parser third-party direct
runtime dependency; C1.3B OOXML/CSV support is stdlib-only. Its `mdurl==0.1.2`
dependency is exact-locked, and both are MIT-licensed.

## Runtime

Required:

```text
RAG_WORKER_DATABASE_URL=postgresql://...
```

Safe defaults:

```text
RAG_WORKER_DISPATCH_ENABLED=false
RAG_WORKER_JOB_STAGES=
RAG_WORKER_POLL_SECONDS=1
RAG_WORKER_RESCAN_SECONDS=30
RAG_WORKER_OUTBOX_LEASE_SECONDS=30
RAG_WORKER_JOB_LEASE_SECONDS=90
RAG_WORKER_HEARTBEAT_SECONDS=30
```

Optional Redis wake-up:

```text
RAG_WORKER_REDIS_URL=redis://...
REDIS_KEY_PREFIX=mm-chat
```

Endpoints on the private listener (default `:8081`):

- `GET /health`: event-loop liveness; this is the container healthcheck.
- `GET /ready`: DB/functions/lock/consumer/projection/Redis components.
  `projection=not_ready` is expected before migration `011` and is not a core
  dark-run readiness failure.
- `GET /metrics`: Prometheus exposition.

The worker exposes no query-embedding or rerank HTTP routes. Go performs both
query-time Jina operations directly from the enabled Postgres/vault provider
record. Background MinerU allocate/poll and Jina passage embedding call the
scoped Go provider operations with `RAG_SOURCE_GATEWAY_URL` and
`RAG_SOURCE_GATEWAY_TOKEN`; Python never receives reusable provider Keys. See
[`../docs/contracts/rag-query-hybrid-retrieval.md`](../docs/contracts/rag-query-hybrid-retrieval.md)
and
[`../docs/contracts/rag-provider-admin-gateway.md`](../docs/contracts/rag-provider-admin-gateway.md).

## Replay

Dry-run (does not require a DSN):

```bash
rag-replay outbox --id "$EVENT_ID" --expected-error-code PROVIDER_TIMEOUT
```

Execution requires exact failed ID, expected current error code, operator UUID,
and non-empty reason. Job replay also requires a fresh successor UUID:

```bash
RAG_REPLAY_DATABASE_URL='postgresql://...' rag-replay job \
  --id "$JOB_ID" \
  --expected-error-code PROVIDER_TIMEOUT \
  --successor-job-id "$SUCCESSOR_ID" \
  --operator-id "$OPERATOR_ID" \
  --reason 'approved incident replay' \
  --execute
```
