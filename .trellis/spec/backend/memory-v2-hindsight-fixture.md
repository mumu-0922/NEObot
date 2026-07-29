# Memory v2 Hindsight fixture contract

## 1. Scope / trigger

Apply this contract when changing `internal/hindsightfixture`,
`cmd/memory-hindsight-fixture`, `compose.hindsight-fixture.yml`, its wrapper or
topology test, the bound synthetic manifest, or the Hindsight comparison report.

PR13 is fixture-only. It cannot read Live chat/Memory, use a Live Provider,
change Native Memory, or remain running after the comparison.

## 2. Signatures

```text
neo-chat.memory-hindsight-fixtures.v1
neo-chat.memory-hindsight-fixture-report.v1
neo-chat.memory-hindsight-fixture-adapter.v1
```

```bash
cd mm-chat
bash scripts/run-memory-hindsight-fixture.sh
bash scripts/test-memory-hindsight-fixture.sh
```

The adapter boundary is:

```go
hindsightfixture.DecodeManifest(io.Reader) (hindsightfixture.Manifest, error)
hindsightfixture.ManifestContentSHA256(hindsightfixture.Manifest) (string, error)
hindsightfixture.ValidateGoldenBinding(
    hindsightfixture.Manifest,
    memoryeval.GoldenSet,
    string,
) error
hindsightfixture.DeriveBankID(string, string, Mode, string, string) (string, error)
(*hindsightfixture.Runner).Run(...) hindsightfixture.Report
```

## 3. Contracts

- Pin Hindsight `0.8.5`, commit
  `e5b4c52d7ea9bf8ed45ba910f3ad4f92a7bb824a`, full API image digest
  `sha256:35d88f6fc2d63ba37e8118dc02945097bf34e4ad04d4f3299e3c426db72c04ba`,
  and pgvector PG17 digest
  `sha256:d2ef61f42ef767baa5a1475393303cc235bcd92febd9d7014eddb48b41f3bad0`.
- Use only the audited REST configure/retain/recall/document-delete/bank-delete
  contract through Go `net/http`; do not vendor upstream or import a generated
  SDK.
- Fixtures are bounded and strict, content-hash bound, synthetic-only,
  no-real-user-data, no-sensitive-data, and promotion-ineligible. Unknown,
  duplicate, trailing, or caller-controlled endpoint/bank/key/DB fields fail.
- The checked-in Golden must bind the manifest hash. A frozen corpus must pass
  `memoryeval.ValidateGoldenAdmission`; the ten-case draft cannot claim formal
  evaluation.
- Derive opaque bank IDs with HMAC over the ephemeral key, manifest hash, mode,
  fixture alias, and user alias. Derive document IDs independently. Neither ID
  appears in the report.
- `end_to_end` uses raw synthetic events and Hindsight mock extraction;
  `retrieval_only` uses frozen canonical facts and `chunks`. Both use only local
  embedding/reranking on an `internal: true` network. Keep the mock LLM
  provider/model in static server environment; bank PATCH may change only the
  hierarchical extraction/audit fields and must not attempt static overrides.
- Reject secret/untrusted fixture rows before HTTP. Retain then document-delete
  deleted rows before recall. Bind every recalled logical ID to its fixture
  owner and fail closed on unknown/cross-bank/deleted/secret/untrusted results.
- Keep the comparison outside frontend, Go HTTP API, migrations, Native
  canonical/projection/jobs, chat prompt, Usage, Activity, and reader flags.
- Output only content-free IDs/statuses/counts/hashes/version/latency and bounded
  error codes. Never output query/content/text/raw score/trace/bank/document
  IDs/credentials/DB URLs/Provider requests/raw errors.
- Run in a random independent Compose project with a dedicated database/role,
  API key, network, volume, and bounded services. Publish no host port and join
  no Native network. Set Hugging Face/Transformers offline mode, disable model
  telemetry/tracking, and force LiteLLM to its local model-cost map; the private
  network remains the independent zero-egress boundary. The runner is
  read-only, drops all capabilities, and has `no-new-privileges`.
- On every exit path, destroy the exact PR13 project with volumes, delete the
  credential temp directory, and prove zero project container/network/volume.
  Never use main-project `down -v` or touch protected runtime paths.
- A report is evidence only. It cannot retain an instance, promote a reader, or
  authorize a real trial.

## 4. Validation / error matrix

| Condition | Required result |
| --- | --- |
| Policy declaration omitted, true promotion, real/sensitive flag, hash drift | Reject before HTTP. |
| Unknown endpoint/bank/credential/database field | Reject as unknown JSON. |
| Wrong API key | Content-free `unauthorized` failure, then teardown. |
| Timeout/cancel/5xx/malformed/oversized response | Bounded failure code; no raw body; Native chat unchanged. |
| Recalled ID belongs to another fixture or forbidden state | Fail closed without exposing Hindsight text. |
| Report contains fixture/query plaintext or ephemeral key | Refuse publication, then teardown. |
| Existing report path | Refuse overwrite. |
| Bank delete fails | Mark failure; whole project/volume destruction remains mandatory. |
| Compose down or zero-object proof fails | Run fails; do not claim instance deletion. |
| Hindsight wins the draft/formal comparison | Preserve report only; request a separate decision. |

## 5. Good / base / bad

- **Good**: both profiles consume only the hash-bound draft, emit content-free
  reports, and leave zero project runtime object after scoped teardown.
- **Base**: strict hash mode and offline `httptest` tests run without Docker or
  any network; the draft remains promotion-ineligible.
- **Bad**: mount `.env.single-server` or Live Memory, accept a caller bank/API
  URL, publish a port, call a hosted Provider, log a raw response, reuse a PR13
  volume for a trial, or keep the comparison database after reporting.

## 6. Tests required

- Strict manifest/policy/hash/Golden binding and opaque-ID derivation tests.
- End-to-end/retrieval-only content selection, secret/untrusted zero-retain,
  scope tags, cross-bank result, deleted document, and mandatory bank cleanup.
- `httptest` contract coverage for auth, bank mismatch, timeout/cancel, 4xx/5xx,
  duplicate/unknown/trailing/malformed/oversized responses, and sanitized errors.
- Compose render assertions for digest, profile, private network, no ports,
  fixed mounts, offline metadata fences, resource limits, runner hardening,
  and no Native authority.
- Container run proving both report paths and exact project teardown.
- `go test -race ./internal/hindsightfixture ./cmd/memory-hindsight-fixture`, all
  backend tests, `go vet ./...`, backend image build, and full standalone gate.

## 7. Wrong vs correct

### Wrong

```yaml
networks:
  hindsight-private:
    internal: true
environment:
  HINDSIGHT_API_LLM_PROVIDER: mock
```

An internal network alone makes an accidental Hugging Face/LiteLLM metadata
request fail during startup; it does not force libraries to use their local
cache. Likewise, sending `llm_provider` or `llm_model` in the bank PATCH fails
because Hindsight treats them as static server fields.

### Correct

```yaml
networks:
  hindsight-private:
    internal: true
environment:
  HF_HUB_OFFLINE: "1"
  TRANSFORMERS_OFFLINE: "1"
  LITELLM_LOCAL_MODEL_COST_MAP: "True"
  HINDSIGHT_API_LLM_PROVIDER: mock
  HINDSIGHT_API_LLM_MODEL: mock
```

Keep the static LLM identity in the isolated server environment, set the full
offline metadata fence, and PATCH only `retain_extraction_mode` plus
`audit_log_enabled`. The private network then remains an independent egress
denial rather than the primary cache-selection mechanism.
