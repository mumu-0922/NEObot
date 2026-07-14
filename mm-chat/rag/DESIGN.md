# Phase 15.2B Durable Worker Design

## Scope

This package implements durable orchestration, not RAG functionality. The
authority chain is:

```text
Go authority transaction
  -> knowledge_outbox / generation-bound processing job
  -> PostgreSQL SECURITY DEFINER state-machine functions
  -> Python poll/plan/heartbeat/retry/replay orchestration
```

Python treats function results as leased snapshots, never as permission to
invent ACL, consent, governance, generation, or projection state.

## Threat model and trust boundaries

- Outbox rows, job rows, Redis messages, handler failures, and database error
  text are untrusted. Only validated identifiers, allowlisted stages, canonical
  plans, and stable error codes cross orchestration boundaries.
- PostgreSQL functions are the sole mutation authority. The worker credential
  must have function execution grants only; compromise of the Python process
  must not permit direct authoritative-table DML or DDL.
- Redis may be lost, delayed, duplicated, forged, or flushed. A message carries
  no event identity or authority and can only accelerate the next Postgres scan.
- DSNs and replay credentials are secret input. They are never emitted in logs,
  metrics, readiness responses, CLI output, or exception text.
- Replay is a privileged operator boundary with a separate DSN, exact failed
  identity/error CAS, explicit execution flag, and durable database audit.
- Known residual risk: this package cannot enforce database GRANTs or container
  egress policy. Migration permission tests and deployment network policy remain
  mandatory promotion gates.

## Phase 15.2C C0 provider-contract intake

Provider wire evidence is isolated from production code. A closed Draft 2020-12
JSON Schema, strict duplicate/NaN/secret/placeholder semantic checks and RFC
8785 hashes live under `tests/`; public MinerU/Jina evidence is explicitly
`lifecycle.state=draft` with unresolved facts. `require_frozen()` rejects drafts,
synthetic fixtures, blockers, unknown facts, missing independent reviewers and
hash drift.

The in-memory Fake Provider uses Starlette with `httpx.ASGITransport`; it opens
no listener and stores only method/path, header names, body size and body hash.
It never stores authorization values or request bytes. These tools prove that a
future adapter can replay a reviewed contract, not that external processing is
authorized. Production registries remain empty until the independent C-stage
promotion gates pass.

### Provider Capture Harness boundary

`tools/provider_capture.py` is an operator development tool, not a runtime
adapter. Its default path emits a canonical redacted plan and performs zero
network or filesystem writes. Explicit execution uses only process-environment
Jina/MinerU keys, an exact HTTPS host/port/path allowlist, disabled environment
proxy trust and redirects, one connection, no retries, fixed timeouts, bounded
streaming responses, strict UTF-8/JSON, and synthetic inputs generated in code.
If direct WSL egress is unavailable, the only proxy path is the dedicated
`PROVIDER_CAPTURE_PROXY_URL`: it must be an uncredentialed literal private or
loopback HTTP address with an explicit port. Generic proxy variables remain
ignored, the validated proxy is never recorded, and Provider TLS verification
and every target/response gate remain active.

Jina execution is exactly two passage embedding calls (1024 and 2048) plus one
two-document rerank call. The original MinerU execution is a deliberately staged v4
local-upload Submit only: signed upload and polling budgets are zero. Response
loss becomes `unknown_submission` and is never retried. Evidence is a closed v1
canonical JSON snapshot containing only allowlisted metadata, shapes, counts,
finite scores and hashes; it excludes vectors, text, request IDs, MinerU IDs,
URLs, error detail, response bytes and header values. New output directories and
files are private, no-overwrite and atomically written through walked
no-symlink parent directory FDs and direct-child relative syscalls. A MinerU
`unknown_submission` evidence write returns nonzero so automation cannot report
the Capture plan complete.

The separate `provider_capture_mineru_lifecycle` path preserves v1 and adds a
closed Evidence v2 chain with fixed `1/1/60/1` Allocate/Upload/Poll/Download
budgets. Provider-derived URLs pass exact documented authority/path gates before
use; Upload/Download never inherit Auth, Cookie, redirects, or caller targets.
Poll identity/state shapes are closed, and Result ZIPs are bounded and checked
without extraction for traversal, symlink, duplicate, encryption, compression,
CRC, size, entry count, and required artifact presence. Only fixed counts,
state, hashes, booleans, and the optional closed `transportFailureClass` enter
Evidence; dynamic URLs/queries, IDs, errors, entry names/content, and response
bytes remain memory-only. This CLI has not made a successful end-to-end Provider
Capture: its first authorized run recorded a legacy `unknown_download`, and its
second independently authorized run recorded `connect_error` after successful
Allocate/Upload/Poll. A credential-free probe isolated the Private Proxy/CDN
TLS path, and an Owner-authorized all-direct run advanced to `download_failed`.
Future failed Evidence may record only a closed `downloadFailureClass`; actual
status, header values, body, ZIP names/content, exception messages, requests,
URLs, and identifiers remain forbidden. No result promotes a Fixture or Runtime
handler automatically. A later diagnostic located the Gate at `archive_invalid`;
an optional closed `archiveFailureClass` may identify only a structural/security
category or missing required Artifact, never an Entry name, content, or raw ZIP
metadata.

Provider-profile Role mapping keeps Cloud v4 `layout.json` equivalent to the
local/open-source `middle.json/*_middle.json` role without persisting either
name. The repaired Capture completes acquisition with all four Role booleans;
Schema/content validation and Canonical IR promotion remain separate blocked
gates.

The harness cannot edit fixtures, freeze the External Gate, derive/apply
Governance or enable production registries. Docker copies only `src/`, and no
project script exposes the harness. The complete threat model and operator
review/rollback flow are in
`../docs/contracts/provider-capture-harness.md`.

## Phase 15.2C C1.1 offline parser contracts

The `mm_chat_rag.contracts` package contains Draft 2020-12 Closed Schemas for
Canonical IR v2, Source Locator v2, normalization, quality, chunk, manifest,
profiles, protocol headers, corpus provenance, and stable errors. Schema IDs
use the reserved `.invalid` namespace; the test validator resolves every
reference from packaged bytes and has no network retriever.

The narrower Contract Profile accepts only null, booleans, Unicode scalar
strings without NUL, and I-JSON safe integers. Strict parsing rejects duplicate
keys, all float tokens, non-finite constants, BOM, invalid UTF-8, surrogates,
unsafe integers, and non-canonical RFC 8785 bytes before schema validation.
Logical hashes use exact ASCII domain tags plus canonical bytes; time, host,
runtime paths, database UUIDs, object keys, and Provider identifiers are absent
from immutable hash shapes.

Corpus, recipe, hash, license, and Python/Go/Node conformance tooling remains
under `tests/` and `tools/`. It freezes test inputs and contracts but does not
claim Native Parser output, accuracy, fresh-container determinism, or Provider
compatibility. Production dispatch and job registries stay empty.

Test-only semantic validators execute the cross-object rules JSON Schema cannot
express: normalization exact-cover and ordering, Locator projection ordering,
reference existence, lineage DAG acyclicity, table-grid bounds, chunk
reconstruction cardinality, manifest counts, and domain-separated aggregate
hashes. Integrated fixtures exercise every normalization transform and Locator
view plus Page/Flow/Block/Table/Cell/Formula/Asset/Provenance and non-empty
Parent/Child overlap paths. They validate frozen contracts; they are not parser
runtime code and are excluded from the wheel.

A separate integrated A–F Hash DAG fixture uses real, recomputed IDs rather
than placeholder hashes and binds Canonical IR, Normalization Map, Source Unit
Resolver, Quality Report, Chunk Manifest, and Canonical Manifest bytes/counts/
hashes. Projection fixtures prove non-separator text exact-cover, deterministic
Map-to-Locator views, legal clipping boundaries, and Child-to-Parent ordered
Fragment/View subsets. Synthetic MinerU fixtures model `layout` and `middle` as
two distinct single-role artifacts; a test-only pair validator reconciles their
source/profile/page/geometry contracts without claiming live Provider wire or
Canonical IR output.

The wheel verifier checks an already-built archive for all 18 schemas, resource
loader, and module documentation while rejecting tests, tools, extra contract
files, duplicate members, links, and unsafe paths. Building remains an explicit
operator/CI step so verification never triggers dependency resolution.

## Phase 15.2C C1.2 offline router and sandbox protocol

### 1. Scope / Trigger

C1.2 is the executable security boundary between untrusted Source bytes and a
Native Parser. It implements format admission, MMCP v1 transport,
process isolation, resource termination, and owned test-output cleanup. It does
not implement Canonical IR generation, database staging, Provider access,
migration `011/012`, Worker dispatch, or query/runtime promotion.

The critical ordering is:

```text
Controller source stat/hash + bound request
  -> private UDS MMCP frame
  -> Supervisor starts clean interpreter in a prebuilt process group
  -> Parent opens pidfd and verifies child PID/PGID
  -> one-byte parent/child handshake
  -> Child installs RLIMIT + no-new-privileges + child seccomp
  -> Child reports the compiled-filter hash
  -> only then may Source bytes enter the child
  -> C1.2 route-only outcome; C1.3A plugs in selected Native Parsers here
```

### 2. Signatures

- Console scripts:
  - `parser-sidecar [--socket PATH]`
  - `parser-harness-smoke`
- Controller:
  - `ParserController.invoke(source, invocation_id, declared_mime=None,
declared_extension=None, cancelled=None) -> ControllerOutcome`
- Router:
  - `route_source(source, declared_mime=None, declared_extension=None) ->
RouteDecision`
- Output root:
  - `OwnedOutputRoot.create(parent=/run/mm-chat-parser-harness) ->
OwnedOutputRoot`
  - `write_artifact(relative_path, content)`
  - `scavenge_stale_roots(parent, stale_after_seconds)`
- Compose profile:
  - `docker compose -f mm-chat/compose.single-server.yml --profile parser-c1
up ... parser-harness-smoke`

### 3. Contracts

MMCP v1 is exactly one request and one response per Unix-socket connection:

```text
4  bytes  "MMCP"
1  byte   protocol major = 1
1  byte   frame type = 1 request | 2 response
2  bytes  reserved = 0
4  bytes  unsigned big-endian JCS header length, <= 16384
8  bytes  unsigned big-endian body length
N  bytes  exact canonical integer-only JCS header
M  bytes  exact body
```

`requestBindingHash` is frozen as:

```text
SHA256(
  ASCII("mm-chat.parser-request-binding.v1\n") ||
  JCS(request header without requestBindingHash) ||
  raw 32-byte SHA256(source)
)
```

The child receives a closed, Supervisor-generated internal header only after
the process-group/pidfd/filter handshake. It does not inherit arbitrary host
environment variables. Resource values come from the same hash-bound
`ParserHarnessConfig`; the only extra inherited value during tests is
coverage.py's serialized instrumentation config, and coverage.py is absent from
the runtime image.

Deployment invariants:

- Sidecar UID/GID `10002:10001`; Worker remains `10001:10001`.
- Sidecar is container PID 1 and installs `PR_SET_CHILD_SUBREAPER`.
- `network_mode: none`, read-only root, all capabilities dropped,
  `no-new-privileges`, `768 MiB`, `1 CPU`, `64 PID`.
- Docker baseline seccomp source and classic-BPF child instructions are hashed
  into the config. Child `clone3` returns `ENOSYS`; Namespace clone bits,
  `setsid`, `setpgid`, `unshare`, `setns`, `ptrace`, and network sockets fail.
- Test output parent is fixed, mode `0700`, and quota-split at
  `512 MiB / 20000` inodes with one active run and a `256 MiB / 10000` artifact
  ceiling.

### 4. Validation & Error Matrix

| Condition                                                         | Outcome                                                      |
| ----------------------------------------------------------------- | ------------------------------------------------------------ |
| Magic/Container conflicts with MIME/extension                     | `FORMAT_MISMATCH`                                            |
| TXT/Markdown/CSV lacks one unique registered hint                 | `FORMAT_AMBIGUOUS`                                           |
| Invalid ZIP/XML/PDF structure, traversal, duplicate, header drift | `INPUT_INVALID`                                              |
| Archive entry/count/ratio/expanded/path limit                     | `ARCHIVE_LIMIT_EXCEEDED`                                     |
| Macro/OLE or macro-enabled OOXML                                  | `ACTIVE_CONTENT_UNSUPPORTED`                                 |
| Scanned/mixed/complex-layout PDF                                  | `MINERU_REQUIRED`, zero body                                 |
| Frame, JCS, binding, hash, deadline, invocation, or tail mismatch | `PROTOCOL_INVALID`                                           |
| Child wall deadline / address-space exhaustion                    | `PARSER_TIMEOUT` / `PARSER_MEMORY_LIMIT`                     |
| Caller cancellation                                               | Controller-local `PARSER_CANCELLED`; never on wire           |
| EOF/reset/Sidecar death                                           | Controller-local `PARSER_SANDBOX_UNAVAILABLE`; never on wire |
| Any descendant remains after main child exits                     | kill/reap group and force Sidecar restart                    |
| No C1.3 parser or no C1.4 Canonical IR                            | wire `FORMAT_UNSUPPORTED`; no fake success                   |
| Marker/lock/device/inode/PID/mode/ledger mismatch                 | refuse cleanup and retain for review                         |

### 5. Good / Base / Bad Cases

- Good: a valid DOCX with matching `.docx` hint routes to `docx` in the child,
  but the current harness returns `FORMAT_UNSUPPORTED` because the DOCX Native
  Parser is not yet active.
- Base: plain UTF-8 bytes without MIME/extension return
  `FORMAT_AMBIGUOUS`; Compose smoke asserts this exact zero-body response.
- Bad: a ZIP with duplicate names, encrypted entries, Local/Central Header
  drift, traversal, macro, or XXE is rejected before any parser fallback.
- Bad: timeout, OOM, double-fork, and bounded fork-bomb probes are terminated;
  descendant presence trips the restart fence even after successful reaping.
- Bad: an unregistered symlink or tampered ownership marker blocks cleanup;
  the harness never follows or recursively deletes it.

### 6. Tests Required

- Corpus router: all 49 frozen route/error expectations, including Archive,
  OOXML/XML, PDF, encoding, and limit negatives.
- Protocol: exact prefix/header/body lengths, canonical JCS, duplicate keys,
  source/binding/result hashes, deadline, response discriminator, tail bytes,
  and Controller-only errors.
- Sandbox: real seccomp install/hash, pidfd/process group, OOM, timeout, cancel,
  double-fork, bounded fork bomb, full descendant reap, and restart gate.
- Output root: admission flock, quotas, `O_NOFOLLOW`, marker identity, unexpected
  child refusal, dead-owner scavenging, and temp-file rollback.
- Deployment: script/Registry fences, UID/PID1/network/seccomp/tmpfs Compose
  invariants, image build, and isolated `parser-c1` smoke.
- Full quality: Ruff, Format, strict Mypy, Pytest with coverage `>=90%`,
  pip-audit, offline wheel verification, and Python/Go/Node JCS equality.

### 7. Wrong vs Correct

Wrong: accept an extension as authority, parse in the Supervisor, fall back to
TXT after a binary error, or return a placeholder success before C1.3.

```python
# Wrong: bytes enter a parser before process isolation and errors are hidden.
try:
    return parse_docx(source)
except Exception:
    return parse_txt(source)
```

Correct: independently bind bytes, route by Magic/Container first, send Source
only after the child handshake/filter hash, and expose one stable no-fallback
outcome.

```python
request = build_request_header(source=source, ...)
request.validate_body(source, expected_config_hash=config.config_hash)
result = SandboxSupervisor(config).route(source, declared_extension=".docx")
assert result.parser_format == ParserFormat.DOCX
# A C1.3 Native Artifact still cannot emit Canonical IR success.
```

### Security limitations

The resource values remain C1 candidates pending worst-case Fresh-container
Corpus tuning. `RLIMIT_NPROC` is charged against a host UID, so unit probes add
bounded test-only headroom when `/proc` exposes only a PID namespace; production
uses the dedicated UID plus the container PID limit. Python/stdlib XML parsing
is preceded by byte-level DTD/Entity/XInclude rejection and has no external
fetch path, but full Native Parser accuracy and deterministic Canonical IR are
C1.3/C1.4 gates. Cgroup-level Sidecar OOM is represented as Controller-local
Sandbox Unavailable; it never becomes a forged wire response.

## Phase 15.2C C1.3A TXT / Markdown / HTML Native Parsers

C1.3A activates only TXT, Markdown, and HTML parsing inside the isolated Child.
It does not activate DOCX/PPTX/XLSX/CSV/PDF/MinerU parsing, Canonical IR,
production Dispatch, database staging, Provider access, or migrations
`011/012`.

```text
Raw Source enters only after C1.2 handshake + Seccomp
  -> BOM / UTF-8 / GB18030 strict decode and compact locator index
  -> fixed TXT | CommonMark+Table | hardened HTML parser
  -> closed parser-native-artifact.v1 internal body
  -> Supervisor checks JCS/length/hash/limit/format/source binding
  -> Sidecar returns zero-body FORMAT_UNSUPPORTED until canonical-ir.v2 exists
```

`parser-native-artifact.v1` preserves Source encoding, exact Raw-byte/Scalar/
Line positions, ordered structure, Attributes, and identity or syntax-decode
Fragments. The Child writes a distinct internal frame: a four-byte canonical
JCS header length, the closed header, an eight-byte body length, the exact
artifact bytes, and EOF. The Parent never decodes or reparses the original
Source when validating that result.

Markdown uses exact-pinned `markdown-it-py==4.2.0` with the CommonMark + Table
profile and no runtime plugin discovery. Raw HTML passes the same active-content
policy as HTML. HTML uses `HTMLParser(convert_charrefs=False)` with explicit
DTD/Entity, external-fetch, active-content, depth, attribute, node, and text
limits. No parser falls back to another format after admission.

The Native Artifact is not an MMCP success payload. MMCP v1 remains frozen to
`canonical-ir.v2`, so a verified internal artifact is non-stageable and stays
off the external wire. The config hash binds all Native limits and the fixed
parser profile; the C1.3A value is
`8a72668218932f6af95d3b6276646304451d7f9ea59ff658ca7887d925e83ea7`.

## Process topology

One process owns:

1. A dedicated PostgreSQL session holding one fixed advisory lock.
2. An async psycopg pool (`min=1`, `max=2`) for short function transactions.
3. A one-second outbox/job poll loop and 30-second exhaustive rescan trigger.
4. An optional Redis Pub/Sub subscriber that only sets an in-memory wake event.
5. A private Starlette/Uvicorn health and Prometheus listener.

The dedicated lock connection is outside the size-two transaction pool because
session locks require stable connection affinity. A second process fails startup
before any claim.

The build and runtime stages both use the absolute virtual-environment path
`/app/.venv`. Console-script shebangs therefore remain valid after the venv is
copied into the final image. `tests/docker-smoke.sh` guards this invariant by
starting `rag-worker` against a disposable readiness-function fixture and by
executing `rag-replay --help` from the final non-root image. The worker smoke also
uses a read-only root filesystem and the production capability/resource limits.

## Dark-run and promotion gate

`RAG_WORKER_DISPATCH_ENABLED=false` is the default. In that state no claim
function is called. If enabled, startup rejects an empty event registry and any
configured job stage lacking a handler. The checked-in production registries
are empty, so Phase B cannot claim parse, passage embedding, or purge work even
under accidental configuration drift. Tests inject synthetic registries without
changing production defaults.

## Outbox algorithm

- Every claim receives a new UUID lock token; one token is never reused.
- Normal poll/wake handles one row. Every 30 seconds, forced rescan drains up to
  a bounded safety limit directly from PostgreSQL pending/expired state. No
  local checkpoint or `MAX(id)` assumption exists, so late-low-ID commits remain
  visible; the applied ledger fences idempotency and result-hash conflicts in
  the atomic Apply/Ack transaction.
- Planning happens outside the claim transaction and performs no provider or
  object I/O.
- A versioned canonical JSON envelope is SHA-256 hashed deterministically.
- Apply + applied-event ledger + ack is one database function transaction.
- Invalid/unknown plans retry, then enter PostgreSQL `status=failed` DLQ at the
  bounded attempt limit. Only stable uppercase error codes cross the boundary.

## Job algorithm

- Global concurrency is one. The claim function receives only configured stage
  allowlists and excludes legacy unbound work at the database contract.
- A fresh lease token fences every claim. Heartbeat runs every 30 seconds for a
  90-second lease.
- Lease loss cancels local execution and never attempts finish with a stale
  token. Success/retry/fail all use the frozen finish function CAS.
- SIGTERM stops new claims. Current work has the configured grace period; after
  it expires local work is cancelled and its database lease is left to expire.

## Health semantics

`/health` only proves process/event-loop liveness. `/ready` reports fixed fields:
database, frozen functions, worker lock, consumer, projection, and Redis.
Projection `not_ready` is legal before `011`; Redis `degraded` never stops polling.
Only `/health` should be used for container restart policy.

## Replay and rollback

Replay is operator-only and defaults to dry-run. Execution requires exact ID,
expected error code, operator UUID, and reason. Job replay creates a caller-chosen
successor ID and does not revive the failed lease. Rollback is stopping the
worker process/profile; no schema downgrade or state mutation is performed by
this package.
