# Hindsight PR13 fixture adapter audit

Date: 2026-07-29

## Frozen upstream identity

- Repository: `https://github.com/vectorize-io/hindsight`
- Version: `0.8.5`
- Commit: `e5b4c52d7ea9bf8ed45ba910f3ad4f92a7bb824a`
- License: MIT
- Full API image:
  `ghcr.io/vectorize-io/hindsight-api:0.8.5@sha256:35d88f6fc2d63ba37e8118dc02945097bf34e4ad04d4f3299e3c426db72c04ba`
- Full API platform manifests:
  - amd64: `sha256:c5867419d631185dc4460470460e74c25677550302292f8d0a96f8dfe6de06c5`
  - arm64: `sha256:12400212171ead3597abc5979746c8d27be54992323e5cb1f8ede9bce9f94ccd`
- PostgreSQL/pgvector image:
  `pgvector/pgvector:pg17@sha256:d2ef61f42ef767baa5a1475393303cc235bcd92febd9d7014eddb48b41f3bad0`

The `0.8.5-slim` API image was also inspected, but it omits the local ML
dependencies required by the no-remote-Provider fixture profile. PR13 therefore
uses the pinned full API image and does not vendor Hindsight source or a
generated client.

## Audited REST boundary

PR13 needs only the following fixed HTTP operations:

```text
GET    /health
PATCH  /v1/default/banks/{bank_id}/config
POST   /v1/default/banks/{bank_id}/memories
POST   /v1/default/banks/{bank_id}/memories/recall
DELETE /v1/default/banks/{bank_id}/documents/{document_id}
DELETE /v1/default/banks/{bank_id}
```

Authentication is supplied by the built-in tenant extension:

```text
HINDSIGHT_API_TENANT_EXTENSION=
  hindsight_api.extensions.builtin.tenant:ApiKeyTenantExtension
HINDSIGHT_API_TENANT_API_KEY=<ephemeral-key>
Authorization: Bearer <ephemeral-key>
```

The adapter deliberately implements this small contract with Go `net/http`.
It does not accept an endpoint, bank ID, database URL, or credential from a
fixture document.

## Offline dual profile

Hindsight `0.8.5` supports the audited `retain_extraction_mode` values
`concise`, `verbose`, `custom`, `verbatim`, and `chunks`.

- `end_to_end` uses `llm_provider=mock` and `retain_extraction_mode=concise`.
  The upstream mock provider creates structurally valid facts without a
  network call, allowing retain -> extraction -> embedding -> recall to run.
- `retrieval_only` uses `retain_extraction_mode=chunks`, which bypasses LLM
  extraction and stores the fixture's frozen canonical fact for retrieval.
- Both profiles use the full image's preloaded local embedding and reranker.
  No OpenAI-compatible URL, key, provider vault, Live chat, or Live Memory is
  admitted.

The private Docker network is necessary but not sufficient by itself. On the
first runtime replay, the full image still attempted Hugging Face metadata
resolution for a preloaded model and LiteLLM attempted to refresh its model
cost map. With no external route, API startup failed before retain/recall.
PR13 therefore also sets `HF_HUB_OFFLINE=1`, `TRANSFORMERS_OFFLINE=1`,
`HF_HUB_DISABLE_TELEMETRY=1`, `DO_NOT_TRACK=1`, and
`LITELLM_LOCAL_MODEL_COST_MAP=True`. These variables are a fail-closed metadata
fence; the private network remains the independent zero-egress boundary.

The first post-startup replay then exposed a separate Hindsight configuration
boundary: `llm_provider` and `llm_model` are static server fields and the bank
configuration API rejects attempts to override them. PR13 therefore fixes both
to `mock` in the isolated API environment and uses the bank PATCH only for the
hierarchical `retain_extraction_mode` and `audit_log_enabled` fields. This keeps
profile selection explicit without relying on an unsupported override.

The default local embedding model is `BAAI/bge-small-en-v1.5`; the default
local reranker is `cross-encoder/ms-marco-MiniLM-L-6-v2`. Both are primarily
English models. Chinese and mixed-language results must therefore be reported
as measured fixture evidence and must not be normalized into an optimistic
claim.

## Delete and teardown finding

The audited bank deletion path removes bank-owned documents/chunks, active and
invalidated memory units, entities, links/associations, per-bank vector indexes,
the bank row, and extension-declared extra bank tables. Document deletion
cascades through its chunks, memory units, and links.

API deletion alone is not sufficient evidence that the fixture runtime is
gone. Audit logs, LLM request traces, file storage, asynchronous operations,
graph-maintenance queues, statistics/caches, and container logs can outlive a
bank deletion. PR13 must therefore:

1. use a random, dedicated Compose project;
2. use a dedicated PostgreSQL database/role/volume and an ephemeral API key;
3. call bank deletion as best-effort narrowing before shutdown;
4. execute scoped `down --volumes --remove-orphans` for that exact project on
   success, failure, and signal;
5. remove the credential temporary directory; and
6. verify that the exact project has no remaining container, network, or
   volume.

The main Neo Chat Compose project and `mm-chat/data`, `mm-chat/secrets`,
`mm-chat/backup`, and `mm-chat/.env.single-server` are outside the deletion
boundary. A future real-data trial cannot reuse any PR13 bank, key, database,
role, network, or volume and still requires new explicit authorization.

## Recorded runtime result

The final 2026-07-29 replay executed both profiles against the hash-bound draft.
Each passed 8 of 10 cases. The temporal case returned both current and forbidden
old logical IDs, and the negative case returned its forbidden unrelated logical
ID; both were correctly classified as `forbidden_memory_result`. The remaining
positive, mixed/CJK, multi-hop, scope, secret/untrusted, deletion, and fallback
cases passed. Both profiles declared zero remote Provider calls.

The retained content-free reports are
`20260729T023922Z-2af7d378-end_to_end.json` and
`20260729T023922Z-2af7d378-retrieval_only.json` under
`mm-chat/docs/tracking/hindsight-fixture-reports/`. The wrapper exited non-zero
for the quality mismatch and still removed the exact project's containers,
network, PostgreSQL volume/database/role, bank state, and credential directory.
An independent Docker label check found zero Hindsight fixture runtime objects.
This draft result does not meet promotion or real-trial admission criteria.

A separate `SIGINT` replay interrupted a live runner, exited `130`, and again
left zero project-labeled container, network, volume, or partial report. This
validated the signal cleanup path independently from the quality-failure run.
After the production/fixture image gates passed, the temporary Neo Chat check
images and the pulled Hindsight/fixture-pgvector images were removed as well.
