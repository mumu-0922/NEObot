# Hindsight synthetic fixture comparison

## Status and boundary

PR13 compares the pinned Hindsight `0.8.5` implementation with Neo Chat's
synthetic Memory contract. It is an optional benchmark profile only. It adds no
frontend behavior, HTTP route, database migration, feature flag, worker,
canonical write, prompt input, Usage/Activity record, or active reader.

The checked-in inputs are:

```text
docs/contracts/memory-hindsight-fixture-draft.json
docs/contracts/memory-benchmark-golden-draft-template.json
```

Both declare `syntheticOnly=true`, `containsRealUserData=false`,
`containsSensitiveData=false`, and `promotionEligible=false`. The current ten
cases are draft smoke evidence and are not the human-reviewed frozen 500-case
corpus required for a promotion evaluation.

## Pinned provenance

```text
Hindsight version: 0.8.5
Hindsight commit:  e5b4c52d7ea9bf8ed45ba910f3ad4f92a7bb824a
API image:         sha256:35d88f6fc2d63ba37e8118dc02945097bf34e4ad04d4f3299e3c426db72c04ba
pgvector pg17:     sha256:d2ef61f42ef767baa5a1475393303cc235bcd92febd9d7014eddb48b41f3bad0
```

The Go adapter uses only the audited REST configure, retain, recall,
document-delete, and bank-delete operations. It does not import a generated
Hindsight client.

## Fixture schema and authoring

The manifest schema is `neo-chat.memory-hindsight-fixtures.v1`. Each logical
Memory has an opaque ID, fixture/user alias, optional Project/Conversation
scope, canonical fact, raw synthetic event, RFC3339 occurrence time, and one of:

```text
active | deleted | secret_rejected | untrusted_rejected
```

`secret_rejected` and `untrusted_rejected` rows are negative synthetic
sentinels and never cross the HTTP boundary. `deleted` rows are retained, then
their opaque document is deleted before any recall.

Strict decoding rejects inputs over 8 MiB, duplicate JSON keys, unknown fields,
trailing values, invalid identifiers/scopes/timestamps/states, invalid data
policy, and a changed canonical `contentSha256`. After editing a synthetic
manifest, compute the candidate canonical hash offline:

```bash
cd mm-chat/backend
go run ./cmd/memory-hindsight-fixture \
  -manifest ../docs/contracts/memory-hindsight-fixture-draft.json \
  -print-manifest-hash
```

Update both the manifest `contentSha256` and the Golden
`fixtureManifestSha256`, then rerun the focused tests. Hash mode does not require
Docker, an API key, or a network.

## Dual profiles

| Profile | Retained text | Hindsight extraction | Purpose |
| --- | --- | --- | --- |
| `end_to_end` | `rawEventContent` | local `mock`, mode `concise` | Verify retain -> extraction -> local embedding/rerank -> recall mechanics without a Provider. |
| `retrieval_only` | `canonicalContent` | mode `chunks` | Isolate Hindsight retrieval behavior from extraction differences. |

Both profiles use the full image's local `BAAI/bge-small-en-v1.5` embedding and
`cross-encoder/ms-marco-MiniLM-L-6-v2` reranker. The models are primarily
English; Chinese and mixed-language results are reported without adjustment.
The API also forces Hugging Face/Transformers offline mode, disables model
telemetry/tracking, and forces LiteLLM to its local model-cost map. The Compose
network is independently `internal: true`, so a configuration mistake still
has no external Provider route. The metadata fence is required even with
preloaded model weights because otherwise those libraries attempt online
metadata refresh during startup.

## Run and teardown

Run only the wrapper:

```bash
cd mm-chat
bash scripts/run-memory-hindsight-fixture.sh
```

The wrapper:

1. creates a random dedicated Compose project;
2. generates a 256-bit database password and API key in a mode-`0600`
   temporary directory;
3. pulls the digest-pinned API/database images and builds the dedicated runner
   target;
4. starts only the isolated PostgreSQL and API services;
5. runs both profiles against the exact checked-in read-only mounts;
6. validates that each report contains no fixture/query plaintext or ephemeral
   credential and publishes it through an exclusive hard link; and
7. on success, failure, cancellation, or signal, runs `down --volumes
   --remove-orphans` for that exact random project, removes the credential
   directory, and verifies zero project-labeled container, network, or volume.

Reports default to `docs/tracking/hindsight-fixture-reports/`. `--output-dir`
may select another evidence directory, but protected Native runtime paths are
rejected. An existing report is never overwritten.

The wrapper never runs `down -v` against the main Compose project and never
mounts or deletes `.env.single-server`, `data/`, `secrets/`, or `backup/`.
Destroying the dedicated PostgreSQL volume is the final proof that banks,
roles/database, audits, LLM traces, files, async operations, graph queues,
caches, and logs cannot remain in the comparison instance.

## Report contract

The output schema is `neo-chat.memory-hindsight-fixture-report.v1`. Allowed
evidence includes:

- manifest/Golden IDs and SHA-256 bindings;
- adapter/upstream version, commit, image digest, and profile hash;
- logical Memory IDs, fixture aliases, rank order, case status/error code, and
  latency; and
- retained/deleted/rejected logical IDs and `remoteProviderCalls=0`.

The report has no query, fixture content, Hindsight text, raw score, trace, bank
ID, document ID, API key, database URL, Provider request, or raw upstream error.
It always declares `promotionEligible=false`.

## Recorded draft replay

The 2026-07-29 isolated replay retained these immutable content-free reports:

- `docs/tracking/hindsight-fixture-reports/20260729T023922Z-2af7d378-end_to_end.json`
  (`sha256:b0459d41db66d4f75bbbc24ec455542f7ce4eaf3cc165fa05b09e95a883fa9ce`)
- `docs/tracking/hindsight-fixture-reports/20260729T023922Z-2af7d378-retrieval_only.json`
  (`sha256:3672d675008aa2e2748331f3cc21e340dcd5946ff9e5d11646739052c35d7381`)

Both profiles passed 8 of 10 draft cases and failed the temporal and negative
cases with `forbidden_memory_result`. Cross-bank isolation, secret/untrusted
rejection, document deletion, fallback, and the other positive cases passed.
Both reports declare zero remote Provider calls and remain promotion-ineligible.
The exact random Compose project then had zero remaining container, network, or
volume; no Hindsight runtime instance was retained. This ten-case result is a
smoke comparison, not the formal 500-case benchmark and not authorization for a
real-data trial.

An independent `SIGINT` replay also interrupted the runner and exited `130`;
the wrapper removed its exact project and left zero container, network, volume,
or partial report. This proves the signal path separately from the quality-
failure teardown above.

After all image gates passed, the local fixture runner/check images and the
pulled Hindsight/fixture-pgvector images were also removed. Reproduction must
pull the same pinned digests again; no local Hindsight image or runtime state is
part of the retained evidence.

A future formal comparison requires a newly reviewed frozen 500-case Golden
corpus whose fixture hash matches and which passes
`memoryeval.ValidateGoldenAdmission`. Replacing the fixed Compose mounts is a
separate reviewed source change. Even a passing formal comparison triggers only
a new decision review; it does not preserve this instance or activate a reader.
A real-data trial remains separately authorized and must use an entirely new
key, database/role, bank set, network, and volume.

## Verification

```bash
cd mm-chat/backend
go test -race ./internal/hindsightfixture ./cmd/memory-hindsight-fixture
go vet ./internal/hindsightfixture ./cmd/memory-hindsight-fixture

cd ..
bash scripts/test-memory-hindsight-fixture.sh
bash scripts/verify-standalone.sh --full
```

The Go tests use only `httptest`; they do not contact Hindsight or a Provider.
The topology test renders Compose without starting a service. The operator run
is the explicit container/PostgreSQL purge drill.
