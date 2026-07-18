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

- Good: a valid DOCX with matching `.docx` hint routes to the fixed DOCX Native
  Parser and yields a verified internal Artifact, while MMCP still returns
  `FORMAT_UNSUPPORTED` because C1.4 Canonical IR is not active.
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
uses hardened source-aware Expat with DTD/Entity/PI/XInclude rejection and no
external fetch path; PDF/MinerU Native processing and deterministic Canonical IR
remain C1.3C+/C1.4 gates. Cgroup-level Sidecar OOM is represented as Controller-local
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

## Phase 15.2C C1.3B DOCX / PPTX / XLSX / CSV Native Parsers

C1.3B upgrades the internal DTO to `parser-native-artifact.v2` and activates
CSV plus three OOXML formats without changing the MMCP success contract. Text
formats retain one decoded Raw File Source Unit. OOXML uses a binary Source Unit
`0`, then positive Part Units ordered by canonical URI unsigned UTF-8 bytes;
all Part-local Locators carry their Source Unit ordinal.

Router and Parsers share exactly one `ValidatedOpcPackage`. Admission reconciles
EOCD, Local/Central Header, optional Data Descriptor, CRC and sizes; rejects
ZIP64, nested archives, path/case/percent collisions, Macro/OLE, invalid Content
Types, open Relationship graphs, DTD/Entity/PI/XInclude, and package-wide
resource overflow. Internal Relationships must resolve to admitted Parts;
External targets remain non-dereferenced metadata and are never fetched.

CSV uses a fixed comma/double-quote FSM with CRLF/LF/CR and no Sniffer or Header
inference. DOCX preserves paragraph/list/table/note structure. PPTX preserves
slide order, shapes, tables, notes and exact-rational geometry. XLSX preserves
sheet/row/cell/shared-string/formula/cached/merge/hidden structure without
executing formulas. These parsers add no third-party runtime dependency.

Artifact v2 remains Child-internal and non-stageable. The Sidecar discards it
and returns zero-body `FORMAT_UNSUPPORTED` until C1.4 emits verified
`canonical-ir.v2`. The C1.3B Config Hash is recorded only after final review and
all gates freeze the implementation bytes:
`6251a7a71ec35d7d55e030b8ca1ef49da8995257734a76e8cd6864c25d88d8c3`.

## G11.9D.1 Structure-aware chunk planning

`mm_chat_rag.structure_chunking` is a pure planning boundary between validated
parser structure and future Canonical IR/Chunk Manifest projection. It accepts
closed structural text units and emits only unit ordinals plus UTF-8 byte
ranges. This prevents the planner from inventing source locators or acquiring
filesystem, database, network, provider, clock, or randomness authority.

Parents are section-bounded and target 1,200–1,600 estimated tokens with a
2,000 hard cap. Children target 300–500 with a 650 hard cap and reuse exact
adjacent atom ranges for bounded overlap. Table rows, headings, code, and
formulas remain atomic while within the target maximum. All token counts are a
versioned `ceil(utf8_bytes/4)` planning estimate, not provider-tokenizer truth.

This slice deliberately has no runtime caller. D.2 must map the plan back onto
validated Native/MinerU blocks and clip existing locators while satisfying the
current lineage/hash validators. D.3 alone may verify and atomically promote a
new Index Generation; planning cannot mutate the active generation.

## G11.9D.2.1 Native structural artifact projection

`mm_chat_rag.native_structure_artifacts` turns a source-bound `NativeDocument`
into Canonical IR v2 and Chunk Manifest v2 without changing the live parser
route. A pre-order structural pass selects each source text exactly once:
standalone text nodes remain blocks, list items aggregate descendant paragraphs,
and table rows aggregate cells. Heading levels form logical-ID ancestry; raw
heading titles are never identity inputs for heading paths.

Each block retains stable source-unit and structure-owner IDs. Identity text can
be clipped to exact raw/scalar/line ranges; syntax-decoded text intentionally
keeps its verified coarse range. The D.1 planner ranges are then rebound to
block-relative spans and clipped locators. Parent/Child content, hashes,
joiners, global Child ordinals, and overlap metadata are deterministic and pass
the packaged schemas plus the existing Postgres projection builder.

This remains a proof boundary, not a promotion boundary. The production
`NativeSandboxParserGateway` continues to emit its old baseline until MinerU
mapping, a versioned Search Profile, new-generation staging, real Jina passage
embedding, verification, and atomic cutover are completed in later slices.

## G11.9D.2.2 MinerU structural artifact projection

`mm_chat_rag.mineru_structure_artifacts` consumes the decoded and digest-bound
archive mapping input. The closed boundary accepts contiguous
synthetic `pages[].elements[]` geometry or observed live-provider `pdf_info[]`
pages, maps known text-bearing kinds, renders table rows/cells deterministically,
and fails on unknown text-bearing elements. Non-text images remain outside
retrieval text.

Canonical pages and block/chunk locators preserve the admitted page index and
half-open BBox. Chunk clipping narrows canonical byte anchors but never invents
finer PDF coordinates. The D.1 planner supplies Parent/Child ranges and exact
overlap under the shared structure profile; schema and Postgres DTO projection
tests prove the boundary.

Native and MinerU mapper identities remain distinct, but their Chunk Manifests
must use the shared `STRUCTURE_CHUNK_PROFILE_HASH`. An Index Generation binds
one `knowledge_index_profiles.chunk_profile_hash`; provider-specific hashes
would make mixed PDF/DOCX staging impossible and are forbidden.

The `pdf_info[]` path combines `para_blocks` and `discarded_blocks`, joins
`lines[].spans[].content`, orders blocks by BBox/index, and scales PDF points to
milli-points. This does not assert every future MinerU version emits this shape;
unrecognized text-bearing shape still fails closed.

## G11.9D.2.3a Candidate generation rebuild allocation

Migration `028_structure_generation_rebuild_allocator` introduces one
`SECURITY DEFINER` function owned by `rag_projection_owner`. The Go runtime
supplies deterministic IDs, request hashes, shared profile hashes, and the
complete active-document allocation set. Under the corpus-head lock, the
function rejects another building/verified candidate and verifies exact set
coverage before creating any durable rebuild state.

The function clones the active embedding/rerank/search provider configuration,
binds the new Index Profile to the shared Native/MinerU structure chunk hash,
creates a `building` generation plus projection state, and allocates one
staging materialization and pending `parse/reprocess` job per document. Parse
governance authority is inherited from the document's latest admitted parse
job. No promotion function or active-head update is reachable from this
boundary.

This is allocation, not processing or cutover. The current active generation
remains authoritative while later slices process and verify the candidate.

## G11.9D.2.3b Candidate structure parse projection

Migration 029 adds a `SECURITY DEFINER` read boundary that resolves the bound
Index Profile hash only for the current processing lease, worker, lease token,
generation, and staging materialization. `AuthorityRoutingParserGateway` then
selects either the existing baseline parser pair or the shared-profile
structure parser pair; it never trusts a caller-supplied profile and rejects
unknown profile/processor identities.

Native structure parsing reuses the sandbox admission result before calling
the deterministic Native artifact builder. MinerU structure parsing reuses the
source-hash-bound result archive before calling its deterministic builder. The
existing Postgres parse-projection function remains the only persistence
boundary and emits pending passage-embedding work after success.

Migration 030 corrects replay timestamp construction by using one database
timestamp for the successor job and replay audit row. This prevents a replay's
`available_at` from preceding its later-evaluated `created_at` by microseconds.

The slice was live-proved on a disposable three-document clone with a
parse-only worker: one real MinerU PDF and two Native DOCX projections staged
under the shared hash, PDF blocks retained page-BBox locators, and three
passage-embedding jobs remained pending. The active generation was not changed.
Jina passage embeddings, completeness verification, cutover, and citation
proof remain outside this boundary.

## G11.9D.2.3c Candidate passage embedding completeness

No parallel embedding implementation is introduced. The promoted
`passage_embedding_handler_with_dependencies` fetches generation-bound Child
search rows, calls Jina with `retrieval.passage`, validates exact Child order,
count, 1024 finite values, and float32 hashes, then stages each vector through
the existing least-privilege function.

Per-materialization completeness joins immutable Child/search lineage and
requires every projection to be ready under `jina-embeddings-v4`/1024 before
`knowledge_complete_embedding_and_publish(...)` publishes the materialization,
advances its generation-scoped document head, and commits the leased job. This
path cannot verify or promote the generation.

Disposable-clone proof completed three parse and three real embedding jobs on
their first attempts. The mixed PDF/DOCX candidate exactly covered all current
documents; three materializations and three document projection heads were
published; all three shared-profile Children had ready vectors. The active
generation remained unchanged. G11.9D.3a below computes/freezes the generation
manifest and projection counts and transitions to `verified`; later gates test
deletion fences and perform the only atomic cutover.

## G11.9D.3a Generation completeness verification

Migration 031 introduces one `SECURITY DEFINER` verifier callable by the Go
runtime. It locks the corpus head under an expected revision, rejects the active
generation as a target, and derives all evidence from Postgres rather than
accepting caller counts or a caller manifest.

Coverage compares exact current document/version/file/content tuples against
published candidate materializations. The latest Parse/Embedding pair, parser
artifact profile/status, Blocks, generation-scoped document heads, Parent/Child
containment, shared chunk profile, locator summaries, ready Jina 1024 vectors,
and immutable lineage joins must all close. Artifact sets transition from
staging to verified in the same transaction.

The manifest uses a versioned domain and hashes stable ordered row digests for
materializations/artifacts, Block locators/content, Parent locators/content, and
Child lineage/vector hashes together with generation/profile/build/count
inputs. On success, generation and projection state receive the same manifest;
counts are frozen and status becomes `verified/ready`. Replaying a verified
candidate recomputes everything and must match exactly.

Live proof returned 3 documents, 10 Blocks, 3 Parents, and 3 Children with an
identical immediate replay. Transactionally removing one ready vector failed
closed and rollback restored the verified state. The active generation/head
were unchanged. D.3c alone may promote and exercise live citations.

## G11.9D.3b Deletion and failed-candidate fencing

Migration 032 replaces the permissive promotion check with a locked verifier
replay. Promotion first locks the expected corpus head, binds the candidate's
persisted chunk-profile and manifest, and invokes the generation verifier in
the same transaction before any active-generation state can move. The existing
delete path locks that same head before tombstoning document/version rows, so
the two operations serialize instead of observing incompatible snapshots.

If the recomputed corpus no longer matches, promotion preserves the active
generation and returns the verifier's closed error. The new fail function then
locks the same head plus exact candidate state and atomically records
`verified/ready -> failed/failed`. Identical replay is idempotent; conflicting
failure codes are rejected. Because only `building|verified` rows consume the
single-candidate slot, a fresh rebuild can start immediately.

Live proof covered both delete-before-promotion and a real lock race. In the
race, promotion waited 1,908 ms behind the delete transaction and then failed
on stale coverage. Two verified candidates were failed and replayed without
moving the active head, and both failures permitted the next replacement
allocation. Migration 032 grants only fail rollback to Go and explicitly
revokes promotion; D.3c owns the first successful promotion, citation proof,
and old-generation rollback exercise.

## G11.9D.3c Atomic cutover and one-step rollback

Migration 033 exposes the already hardened promotion function to the Go
runtime. Promotion retains D.3b's head lock and in-transaction D.3a verifier;
it atomically retires the previous generation, activates the verified structure
generation, and advances the corpus/head revisions. Retrieval and hydration
already bind every reference to the active head, so no query-side generation
switch was added.

Rollback is deliberately asymmetric. The active generation's D.2.3 allocation
snapshot must name the target as its exact `sourceGenerationId`; both persisted
manifests and the expected head must match. Each current document/version/file/
content tuple must still resolve through that target's published head to at
least one complete Parent/Child/ready Jina vector. Extra historical rows remain
legal because the existing authorization, visibility, processing-revision, and
deletion fences already exclude them from retrieval. Success retires the new
generation and restores its source; returning to the structure generation then
requires a fresh rebuild rather than an unsafe toggle.

The real clone promoted 3 documents / 10 Blocks / 3 Parents / 3 Children and a
live `gpt-5.6-sol` answer cited the new generation's exact Parent and Child.
Removing one old ready vector transactionally rejected rollback and restored
the active state. Valid rollback advanced the head again; direct retrieval and
a second real answer cited the restored generation, while stale replay failed.

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
