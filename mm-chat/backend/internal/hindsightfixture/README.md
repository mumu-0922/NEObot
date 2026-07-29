# Hindsight fixture adapter

`hindsightfixture` runs the synthetic-only PR13 comparison against the pinned
Hindsight `0.8.5` REST API. It is benchmark tooling, not a production Memory
engine, reader, writer, or chat dependency.

## Responsibilities

- Strictly decode the versioned fixture manifest with bounded size,
  duplicate-key, unknown-field, trailing-value, data-policy, and canonical
  content-hash checks.
- Bind the fixture to a strictly decoded Memory Golden corpus. A frozen corpus
  must also pass `memoryeval.ValidateGoldenAdmission`; the checked-in ten-case
  draft remains explicitly ineligible for promotion.
- Derive per-run opaque Hindsight bank and document IDs with HMAC. Fixture
  authors cannot provide a bank ID, endpoint, database URL, or credential.
- Drive the audited configure, retain, recall, document-delete, and bank-delete
  REST operations with bounded responses and normalized error codes.
- Run independent `end_to_end` and `retrieval_only` profiles while rejecting
  secret and untrusted fixture rows before the HTTP boundary.
- Emit only content-free logical IDs, statuses, counts, hashes, versions, and
  latency. Query text, fixture text, raw scores, traces, upstream errors,
  credentials, and bank IDs are not report fields.

## Runtime

The only supported operator entrypoint is:

```bash
cd mm-chat
bash scripts/run-memory-hindsight-fixture.sh
```

The wrapper supplies the two checked-in synthetic documents, creates an
ephemeral API key and database password, starts a random isolated Compose
project, runs both profiles, publishes exclusive content-free reports, and
destroys that project on success, failure, or signal.

Direct command execution is intended only for offline tests and the dedicated
runner image:

```bash
mm-chat-memory-hindsight-fixture \
  -manifest /fixtures/manifest.json \
  -golden /fixtures/golden.json \
  -mode retrieval_only
```

`HINDSIGHT_FIXTURE_API_KEY` is injected by the wrapper. The command's API URL is
compiled to the private Compose service name and is not caller-configurable.

## Files

```text
client.go       audited net/http REST adapter and sanitized failures
manifest.go     strict fixture decoder, hash, policy, and Golden binding
opaque.go       HMAC bank/document identifier derivation
runner.go       dual-profile retain/delete/recall orchestration and report
types.go        versioned input/output types and pinned upstream identity
*_test.go       offline httptest, policy, isolation, deletion, and failure tests
```

See [`DESIGN.md`](./DESIGN.md) and the operator contract at
[`../../../docs/contracts/memory-hindsight-fixture.md`](../../../docs/contracts/memory-hindsight-fixture.md).
