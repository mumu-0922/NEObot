# Native Memory regression capture

`memorycapture` executes the protected 500-case machine regression corpus
through Neo Chat's production v1 lexical and v2 hybrid reader seams. It turns
transient ranking surfaces into strict, non-promotional observation artifacts
without changing prompt, Usage, feature flags, or production data.

## Responsibilities

- replay and byte-verify the fixed regression fixture/corpus/audit/manifest;
- map opaque fixture identities to deterministic ephemeral UUIDs;
- seed a fresh, marked PostgreSQL 17 database and build BGE-M3 projections;
- call the production `usermemory` v1 and hybrid readers;
- decorate repository and Provider seams to capture RRF, final, and Provider-
  sent Memory IDs before production diagnostics discard them;
- assemble strict regression observations, content-free run manifests, and
  exclusive private output bundles;
- separate the zero-network `fake_protocol` profile from live SiliconFlow
  reader-quality evidence.

The package has no Golden admission, Holdout, profile-promotion, prompt-
injection, or active-reader authority.

## Main entrypoints

| API | Purpose |
| --- | --- |
| `LoadProtectedRegression` | Regenerate and byte-verify all protected inputs. |
| `BuildFixtureIndex` | Create deterministic alias/UUID authority maps. |
| `SeedEphemeralDatabase` | Materialize synthetic users, scopes, messages, Memories, and lexical projections. |
| `PopulateProjectionVectors` | Populate fixed 1024-dimensional BGE projection vectors through the Provider interface. |
| `CaptureProfiles` | Execute v1 and v2 against the same reset synthetic state. |
| `AssembleRegressionObservations` | Bind ordered captures to the strict regression schema. |
| `BuildRunManifest` | Create a content-free, explicitly non-promotional run record. |
| `PublishArtifactsExclusive` | Publish a private bundle without overwriting evidence. |

## Operator entrypoint

Use the isolated wrapper from the product root rather than invoking this
package directly:

```bash
bash scripts/run-memory-regression.sh \
  --provider-mode fake_protocol \
  --cost-basis /secure/memory-regression-cost-basis.json \
  --output-dir /secure/memory-regression-runs
```

`fake_protocol` validates SQL, capture, evaluation, publication, and teardown
only. A live comparison requires a separately authorized mode-`0600`
SiliconFlow key file; it is always labelled `native_v2_hybrid` while fake
output is labelled `native_v2_hybrid_fake_protocol`.

## Tests

```bash
cd mm-chat/backend
go test -race ./internal/memorycapture ./cmd/memory-regression-capture

# PostgreSQL 17 + pg_textsearch 1.3.1 + pgvector 0.8.5
MM_CHAT_TEST_DATABASE_URL=... \
  go test -run TestNativeMemoryRegressionLivePostgres ./internal/memorycapture

cd ..
bash scripts/test-memory-regression.sh
```

See [DESIGN.md](DESIGN.md) and
[`docs/contracts/memory-benchmark-workflow.md`](../../../docs/contracts/memory-benchmark-workflow.md).
