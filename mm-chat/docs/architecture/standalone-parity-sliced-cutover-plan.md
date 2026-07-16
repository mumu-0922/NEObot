# Standalone Parity Sliced Cutover Plan

## Authority

This is the active remaining-work plan for making `mm-chat/` the only
standalone project root. It supersedes scattered remaining-work sequencing in
older roadmap notes, while preserving older architecture/contract documents as
supporting references for domain details.

Active process log: [`../tracking/standalone-parity-sliced-process.md`](../tracking/standalone-parity-sliced-process.md).

Owner directive captured on 2026-07-15:

- collect all remaining unfinished migration work into one new plan;
- migrate one bounded group at a time;
- test the migrated group immediately;
- do not repeatedly run or depend on one large full-suite pass for every small
  change;
- keep a new dedicated process log for this cutover.

## Non-Negotiable End State

- `mm-chat/` installs, builds, tests, runs, backs up, restores, and deploys
  without the former root project.
- The visible frontend remains the Neo Chat baseline unless the owner approves a
  separate redesign.
- The final runtime is server-only: browser -> frontend edge -> Go -> private
  services/providers.
- Browser IndexedDB/OPFS and `local|server` production authority are removed
  only after equivalent server-backed behavior passes group gates.
- The former root project is deleted only after a separate exact delete plan and
  one-shot owner confirmation.

## Current Evidence Snapshot

### Already Migrated Spine

- Relocated Next.js frontend lives under `mm-chat/frontend/` with its own
  manifest, lockfile, tests, assets, and Docker build context.
- Single-server Compose exposes the frontend on port `18080` and proxies
  `/mm-api` to the private Go backend.
- Go owns health/readiness/version, chat CRUD/SSE, file upload/download,
  browser import, Auth skeleton, Teams APIs, Knowledge control-plane APIs,
  metrics, and readiness.
- Server-mode core chat, files/attachments, plugins context injection,
  reasoning toggle, regeneration, message version switching, chat duplication,
  assistant preset application, smart rename/title generation, related-question
  generation, and Agent/Assistant catalog reads have been implemented and
  tested.

### Remaining Blocking Surfaces

`ChatApp.tsx` still intentionally blocks these server-mode actions:

```text
search toggle
```

The relocated frontend still registers these 11 transitional Next.js routes:

```text
/api/access/verify
/api/chat
/api/chat/execute-code
/api/chat/generate
/api/chat/generate-image
/api/chat/generate-title
/api/chat/related-questions
/api/health
/api/search
/api/voice/synthesize
/api/voice/transcribe
```

Unfinished domains still include Auth UI lifecycle, Teams UI, Knowledge/RAG UI,
Plugin final ownership, Provider Settings/BYOK, Voice, Image, Code Execution,
Search, parser/RAG/citations, production local-mode removal, visual regression,
backup/restore proof, and final clean-copy deletion gates.

## Slice Rule

Every group below must follow this loop:

```text
scope group -> inspect live source/runtime -> define contract -> implement ->
targeted tests -> domain smoke -> record process -> focused commit ->
only then start next group
```

Required per-group evidence:

1. changed files and rollback surface;
2. backend tests for handlers/repositories/contracts when backend behavior
   changes;
3. frontend unit/integration tests for adapters, stores, and UI blocking removal;
4. browser or HTTP smoke for user-visible runtime changes;
5. explicit process-log entry with commands and decisive output;
6. no silent fallback from server mode to browser-local behavior;
7. focused Git commit for the completed group before the next group starts.

Full-suite gates are reserved for domain cutover, release candidates, and final
clean-copy closure. They are not the default loop for every small migration.

## Migration Groups

### G0 — Plan Freeze and Guardrails

Objective: make this plan and its process log the active control plane.

Scope:

- create this plan;
- create the dedicated process log;
- update docs indexes and progress tracking;
- keep older detailed plans as references, not the active queue.

Exit gate:

- plan and process documents exist and are linked from docs indexes;
- progress tracker has group-level checkboxes;
- first process entry records the owner directive and evidence snapshot.

### G1 — Conversation and Message Operations

Objective: remove server-mode blockers for core chat-management actions.

Scope:

- chat deletion;
- chat renaming;
- pinning;
- chat duplication;
- smart rename / title generation;
- message deletion;
- message editing;
- edit branches;
- message retraction;
- message version switching;
- regeneration;
- assistant presets;
- system instruction editing.

Likely backend contracts:

- conversation `PATCH`, `DELETE`, duplicate, pin, title/smart-title endpoints;
- message `PATCH`, `DELETE`, branch/version, retract, regenerate endpoints;
- repository lock-order and ownership checks matching existing chat CRUD/SSE;
- idempotent or conflict-safe semantics for destructive operations.

Frontend work:

- typed server adapters/store actions;
- remove matching `showServerUnsupportedAction(...)` branches only after
  server-backed behavior exists;
- preserve current UI layout and interaction language.

Targeted tests:

- Go repository and handler tests for each mutation class;
- frontend adapter/store tests for success and failure normalization;
- component tests proving the controls call server actions instead of showing
  unsupported toasts;
- one persisted browser smoke: create conversation -> mutate -> reload -> verify
  persisted state.

#### G1 Slice Ledger

- [x] G1.1 Conversation metadata operations: server-backed chat deletion,
      chat renaming, pinning, and system instruction editing.
- [x] G1.2 Message deletion and retraction.
- [x] G1.3 Message editing and edit branches.
- [x] G1.4 Regeneration and message version switching.
- [x] G1.5 Chat duplication and assistant presets.
- [x] G1.6 Smart rename / title generation through server-owned route.

### G2 — Related Questions and Agent/Assistant Catalogs

Objective: replace remaining helper-generation and catalog routes with
server-owned contracts.

Scope:

- title generation route already closed in G1.6; remove legacy
  `/api/chat/generate-title` only during G9 route removal;
- `/api/chat/related-questions`;
- `/api/agents`;
- `/api/agents/{identifier}`;
- assistant preset application closed in G1.5; catalog ownership remains here.

Targeted tests:

- provider request shaping tests;
- catalog static/server implementation tests;
- UI smoke for smart rename, related prompts, and assistant selection.

#### G2 Slice Ledger

- [x] G2.1 Related questions through a Go-owned conversation route.
- [x] G2.2 Agent/Assistant catalog list and detail through Go-owned catalog
      routes.
- [x] G2.3 Frontend API client and services route server-mode G2 calls through
      `/v1/*` instead of transitional Next `/api/*` routes.

### G3 — Auth, Runtime Config, Provider Settings, and BYOK

Objective: make login/session/config/model/provider state server authoritative.

Scope:

- frontend Auth lifecycle wired to Go Auth routes;
- `/api/access/verify` removal path;
- `/api/config` replacement;
- `/api/providers/models` replacement;
- `/api/byok/public-key` replacement;
- Provider Settings / BYOK UI adapters;
- hosted/dev auth mode behavior.

Targeted tests:

- Go auth/config/provider handler tests;
- frontend session bootstrap/logout tests;
- model list and provider capability tests;
- same-origin CSRF/cookie smoke in Compose.

#### G3 Slice Ledger

- [x] G3.1 Go-owned runtime config, server-default provider model list, BYOK
      public-key route, and frontend API-client boundary shells.
- [x] G3.2 Frontend Auth lifecycle wired to Go login/logout/me without
      regressing the local access-password rollback path.
- [x] G3.3 Provider Settings/BYOK UI adapters call the API client instead of
      direct transitional Next routes.
- [x] G3.4 Hosted/dev auth behavior and same-origin smoke verified.

### G4 — Plugin Registry, Install, and Execution Final Ownership

Objective: remove transitional Next plugin ownership and make plugin state
server-governed.

Scope:

- `/api/plugins/list`;
- `/api/plugins/install`;
- `/api/plugins/execute`;
- installed/active plugin validation;
- bounded untrusted tool results;
- final answer context injection through Go stream.

Slice checklist:

- [x] G4.1 Server plugin tool planning and bounded result context: Go owns
      provider-side planning through `/v1/chat/tools/plan`; the browser offers
      only active enabled plugin functions, executes planned calls through the
      hardened plugin execution path, and appends bounded untrusted context to
      the final Go stream.
- [x] G4.2 Plugin registry/list adapter: replace server-mode marketplace reads
      with a typed API-client path and explicit unavailable behavior when no
      registry provider is configured.
- [x] G4.3 Plugin install/custom-manifest adapter: move install validation
      behind the server contract or explicitly keep the transitional route as a
      bounded rollback-only path.
- [x] G4.4 Plugin execute API-client boundary: route execution through the
      plugin API client and isolate the transitional `/api/plugins/execute`
      path behind the contract adapter.
- [x] G4.5a Go plugin execution fail-closed admission: register
      `/v1/plugins`, `/v1/plugins/install`, and `/v1/plugins/execute`; route
      server-mode execution to Go and remove production server-mode fallback to
      `/api/plugins/execute`.
- [x] G4.5b Minimal Go plugin execution sandbox: full manifest payload
      execution, encrypted auth decryption, outbound URL policy, timeout,
      response-size bound, and generic result normalization in Go.
- [x] G4.5c Registry-backed plugin execution finalization: persistent plugin
      registry, id-only execution, audit metadata, and built-in normalizers.
      Local-only transitional executor removal remains deferred to the G9
      clean-copy route-removal gate.
  - [x] G4.5c.1 Go registry id-only bridge: seed built-ins in a Go registry,
        register installed plugin payloads through `/v1/plugins/install`, and
        make server-mode execution send `pluginId/functionName`.
  - [x] G4.5c.2a Durable registry persistence: add Postgres schema/repository,
        wire API startup to use it when `DATABASE_URL` is configured, list
        built-ins/installed plugins from Go, and prevent installed plugins from
        shadowing built-in ids.
  - [x] G4.5c.2b Custom OpenAPI manifest conversion in Go: install
        raw custom OpenAPI JSON, fetch bounded manifest URLs through the Go
        outbound policy, convert supported operations into registry plugins,
        and keep id-only execution working after install.
  - [x] G4.5c.2c Built-in result normalizers in Go: normalize Jina
        markdown extraction, Agnes image/video result envelopes, and Unsplash
        search result arrays before returning `/v1/plugins/execute`.
  - [x] G4.5c.2d Plugin audit metadata beyond installing-user persistence.
- [x] G4.6 Plugin smoke: one installed plugin produces bounded context and a
      persisted final answer through the Go stream.
  - [x] G4.6a Zero-cost in-process smoke harness: real Go chat/plugin HTTP
        handlers install a custom OpenAPI plugin, plan and execute it with a
        fake provider/transport, pass bounded untrusted plugin context into the
        Go stream, and verify persisted assistant completion.
  - [x] G4.6b Live browser/provider smoke: run the same visible flow through
        the deployed frontend with approved provider/plugin credentials.

Targeted tests:

- plugin registry/install permission tests;
- execution sandbox/result-boundary tests;
- frontend active-plugin validation tests;
- browser smoke with one installed plugin producing bounded context.

### G5 — Search and Web-Enrichment Toggle

Objective: make Search explicitly server-owned or explicitly unavailable by
policy; never silently browser-local.

Scope:

- `/api/search`;
- `search toggle` server-mode blocker;
- provider decision for Firecrawl/SearXNG/other approved source;
- degraded unavailable state when no provider is configured;
- optional weather/web enrichment boundaries if tied to search UX.

Current owner note: Search was previously paused. Keep implementation paused
until the owner reopens it, but keep the work item in this cutover backlog.

Targeted tests:

- provider availability/capability tests;
- frontend toggle capability tests;
- no-key unavailable smoke;
- configured-provider smoke when credentials/service exist.

### G6 — Voice, Image Generation, and Code Execution Jobs

Objective: move model/tool jobs out of transitional Next routes and behind
server admission, storage, and audit controls.

Scope:

- `/api/voice/synthesize`;
- `/api/voice/transcribe`;
- `/api/chat/generate-image`;
- `/api/chat/execute-code`;
- unified job admission/rate-limit/cancel/audit behavior where practical.

Slice checklist:

- [x] G6.1 Server-mode fail-closed capability gates: expose disabled
      `voice`, `imageGeneration`, and `codeExecution` capabilities and block
      server-mode service calls from falling through to transitional Next
      routes. Browser-native speech APIs remain local-only UI behavior.
- [x] G6.2 Go job admission contracts for voice synthesis/transcription: register `/v1/voice/transcribe` and `/v1/voice/synthesize` as validating fail-closed admission routes returning `VOICE_JOBS_UNAVAILABLE` until executors exist.
- [x] G6.3 Go job admission contracts for image generation: register `/v1/images/generations` as a strict `modelRef + prompt` validating fail-closed admission route returning `IMAGE_JOBS_UNAVAILABLE`.
- [x] G6.4 Go job admission contracts for code execution: register `/v1/code/executions` as a strict `modelRef + language + code` validating fail-closed admission route returning `CODE_EXECUTION_UNAVAILABLE`.
- [ ] G6.5 Audit/rate-limit/cancel metadata and enabled-provider smoke.
  - [x] G6.5a Admission audit metadata: voice/image/code fail-closed services
        record sanitized job events with kind/status/user/provider/model/language/reason and no prompt/code/text/audio payloads.
  - [x] G6.5b Shared job rate-limit and cancellation gates: register fail-closed `/v1/jobs/{jobId}/cancel` and verify job control routes remain under the global rate-limit middleware.
  - [ ] G6.5c Real voice/image executors with output storage and provider smoke.
    - [x] G6.5c.1 Storage-only result artifact boundary: add a Go
          `jobartifacts` service that validates image/audio result metadata and
          persists executor outputs only through the backend file/object-storage
          boundary, without calling real providers.
    - [ ] G6.5c.2 Real voice executor integration with stored audio artifacts
          and configured-provider smoke.
      - [x] G6.5c.2a Voice executor opt-in seam: add a Go executor interface,
            pass multipart audio into the service, require an explicitly
            configured sanitized admission audit recorder before executor calls,
            and require artifact storage before synthesis executors can run.
      - [ ] G6.5c.2b Real provider-backed voice executor and authorized
            configured-provider smoke.
        - [x] G6.5c.2b.1 OpenAI-compatible voice executor, Go route wiring,
              and gated live smoke harness for `/audio/speech` and
              `/audio/transcriptions`.
        - [ ] G6.5c.2b.2 Authorized configured-provider voice smoke.
        - [ ] G6.5c.2b.3 Free/simple TTS provider selection and smoke: keep
              the Go voice executor interface and `/v1/voice/*` routes as the
              future free hosted TTS API seam. Do not prioritize a local
              Piper-style VPS executor because owner constraints reject extra
              VPS storage/CPU for voice. Browser `speechSynthesis` may remain
              an immediate local fallback and test guard, but closing this item
              requires selecting a free/low-cost hosted API or compatible relay,
              wiring it through the Go executor, storing sanitized artifacts
              when needed, and running an authorized smoke.
    - [x] G6.5c.3 Real image executor integration with stored image artifacts
          and configured-provider smoke.
      - [x] G6.5c.3a Image executor opt-in seam: add a Go executor interface,
            require an explicitly configured sanitized admission audit recorder
            before executor calls, and store generated images through the
            artifact boundary. See `../contracts/media-job-executor-seams.md`.
      - [x] G6.5c.3b Real provider-backed image executor and authorized
            configured-provider smoke.
        - [x] G6.5c.3b.1 OpenAI-compatible image executor plus gated live
              smoke harness.
        - [x] G6.5c.3b.2 Authorized configured-provider image smoke passes
              against an image-capable key/endpoint.
      - [x] G6.5c.3c Go HTTP route wiring: `cmd/api` now constructs the
            image job service with sanitized audit logging, OpenAI-compatible
            executor opt-in, and backend file/object-storage artifact storage
            when the required runtime dependencies are configured.
      - [x] G6.5c.3d Frontend server-mode image adapter and capability
            reopen: `generateImage()` now calls Go `/v1/images/generations`,
            maps returned artifact metadata to server-backed image attachments,
            and downloads bytes only through `/v1/files/{fileId}/content`.
  - [x] G6.5d Code execution sandbox contract before any real executor is enabled: see `../contracts/code-execution-sandbox-contract.md`; runtime remains disabled.
  - [x] G6.5e Live provider smoke authorization gate: add a default-deny
        `providersmoke` gate requiring an explicit enabled flag, exact approval
        phrase, exact target, and run id before any quota-consuming provider
        smoke can run. See `../contracts/provider-live-smoke-authorization.md`.

Targeted tests:

- handler tests for admission, unsupported providers, and output metadata;
- frontend capability gating tests;
- one smoke per enabled provider class;
- fail-closed smoke when provider/config is absent.

### G7 — Knowledge, Document Parsing, RAG, and Citations

Objective: make server RAG production-visible with the owner-locked real
provider profile in
[`g7-rag-citation-cutover-plan.md`](./g7-rag-citation-cutover-plan.md).

Scope:

- Go-owned replacement surfaces for current `mm-chat` legacy Next
  `/api/doc-parse`, `/api/rag/*`, and `/api/chat/rag-queries` routes;
- Knowledge selection DTOs in chat;
- private Python indexing/query services promoted from dark-run to real
  MinerU + Jina + Postgres work;
- Go source reauthorization and citation minting;
- parser artifacts, Postgres projections, tombstones, rebuilds, and
  reproducible reindexing.

Locked standalone profile:

- provider loop: MinerU parser, Jina 1024-dimensional embeddings, Jina rerank,
  and Postgres dense/lexical/exact projection (G7.4 stores the first dense lane extension-independently; pgvector/true BM25 accelerators remain a later reversible search-profile migration);
- PDF scope: all PDF classes in the first round, including scanned and complex
  formula/table PDFs;
- credentials: administrator-owned backend env/Docker secrets first; admin web
  key configuration is deferred;
- indexing trigger: upload/bind auto-enqueues background indexing;
- query scope: only Knowledge collections selected/enabled in the current chat;
- answer policy: Strict Knowledge refuses unknowns; normal chat may degrade only
  with explicit no-Knowledge-evidence metadata;
- citation UI: basic marker/card first; rich PDF highlight remains deferred;
- legacy route strategy: keep `mm-chat` Next RAG/doc-parse routes until G9.

Supporting detailed plans remain authoritative for internals:

- `g7-rag-citation-cutover-plan.md`;
- `phase-15-1-knowledge-control-plane-plan.md`;
- `phase-15-1c-team-services-plan.md`;
- `phase-15-1d-collection-document-consent-plan.md`;
- `phase-15-2-single-server-python-rag-consumer-indexing-plan.md`;
- `phase-15-2b-durable-consumer-plan.md`;
- `phase-15-2c-generation-bound-indexing-plan.md`;
- `phase-15-2c-offline-parser-canonical-ir-plan.md`.

Slice checklist:

- [x] G7.1 Decision lock, runtime inventory plan, and dedicated process log.
- [x] G7.2 Admin provider config and fail-closed readiness.
- [x] G7.3 Provider-backed parser/index profile gate.
- [x] G7.4 Canonical IR to chunks and Postgres projection.
- [ ] G7.5 Worker dispatch, rebuild, delete, and retry loop.
- [ ] G7.6 Private query and Go reauthorization.
- [ ] G7.7 Strict/optional chat answer and basic citations.
- [ ] G7.8 Live MinerU + Jina + Postgres smoke and operational proof.
- [ ] G7.9 G8/G9 handoff and G7 closure checklist.

Targeted tests:

- outbox duplicate/out-of-order/replay tests;
- parser fixture tests;
- retrieval/citation/source-authorization tests;
- deletion/tombstone/rebuild tests;
- tenant isolation and prompt-injection tests;
- strict/optional failure contract smoke.

### G8 — Teams and Knowledge UI Wiring

Objective: wire existing Go control-plane APIs into the current frontend theme.

Scope:

- Teams screens/actions;
- membership and invite lifecycle;
- Knowledge collections/documents/consent screens;
- file-to-document binding UI;
- cross-user/team authorization UX.

Targeted tests:

- frontend adapters for Teams and Knowledge APIs;
- two-user/team isolation tests where supported;
- UI smoke for collection/document lifecycle;
- consent enforcement smoke.

Slice sequence:

- [x] G8.1 Adapter seam only: extend the frontend API client with typed
      `TeamApi` and `KnowledgeApi`, keep local mode fail-closed, route server
      mode through Go `/v1/teams/*` and `/v1/knowledge/*`, and verify route
      shape/capability flags without changing visible UI.
- [x] G8.2 Teams UI shell/actions: Team list/detail, members, invites, leave,
      role update, and revoke flows using the existing theme.
- [x] G8.3 Knowledge collection/document UI shell: collection CRUD, document
      status/list, file upload-to-bind flow, and immediate deletion
      invisibility.
- [x] G8.4 Consent UX: collection and query consent controls for
      administrator env-backed MinerU/Jina processing, with strict fail-closed
      copy.
- [x] G8.5 Frontend isolation smoke: personal vs team visibility and
      selected-chat Knowledge scope through the browser/server-mode path.

### G9 — Data Authority and Legacy Route Removal

Objective: remove production browser-local authority and transitional Next API
routes after their replacement groups pass.

Scope:

- production `local|server` branch removal or hard dev-only fencing;
- IndexedDB/localforage/OPFS production write-path removal;
- explicit one-time import remains supported;
- remove replaced Next `/api/*` handlers;
- ensure no build/runtime path escapes from `mm-chat/` to former root.

Slice sequence:

- [x] G9.1 Route inventory freeze: lock the current transitional Next
      `src/app/api/**/route.ts` surface with a static test before deleting any
      handlers.
- [x] G9.2 RAG/doc-parse route removal: remove replaced `/api/rag/*`,
      `/api/doc-parse*`, and `/api/chat/rag-queries` handlers after Go/RAG
      server-mode callers stay green.
- [x] G9.3 Config/provider/BYOK route removal: retire `/api/config`,
      `/api/providers/models`, and `/api/byok/public-key` from production
      server-mode flow.
- [x] G9.4 Plugin/agent route removal: retire transitional
      `/api/plugins/*` and `/api/agents*` handlers or fence any static catalog
      replacement as server-owned.
- [x] G9.5 Local production authority removal: hard-fence browser-local
      IndexedDB/localforage/OPFS write authority to dev/import-only paths.
  - [x] G9.5a Zustand persistence authority fence: in server mode,
        `getAppDbStorage` and `getBrowserLocalStorage` return no-op storage so
        persisted stores cannot hydrate from or write to IndexedDB/localStorage
        as production state. Explicit browser-import direct `appDb`/OPFS reads
        remain allowed.
  - [x] G9.5b OPFS write/delete authority fence: in server mode,
        `saveToOPFS`, `writeToOPFS`, `deleteFromOPFS`, and
        `deleteOPFSDirectory` throw a typed authority error; OPFS list/read
        remain available for explicit import.
  - [x] G9.5c Direct `appDb` authority sweep: replace direct chat message
        `appDb.setItem/removeItem` calls with runtime helpers that throw in
        server mode; explicit browser-import reads remain available.
- [x] G9.6 Clean-copy preflight: prove `mm-chat/` build/test/runtime no longer
      imports former-root artifacts.

Targeted tests:

- static path/import/build-context gate;
- route inventory gate proving replaced handlers are gone;
- frontend storage authority tests;
- clean server-mode browser smoke.

### G10 — Operations, Visual Regression, Clean Copy, and Delete Plan

Objective: prove standalone closure and prepare the destructive deletion gate.

Scope:

- backup/restore drill for Postgres, object storage, and search/RAG artifacts;
- Compose source-build/restart/rollback evidence from only `mm-chat/`;
- desktop/mobile visual regression and interaction smoke;
- clean-copy install/test/build/run with only `mm-chat/` present;
- exact former-root delete plan with paths, backups, rollback, and owner
  confirmation prompt.

Deployment closure decision recorded on 2026-07-16: the active gate is
source-build Compose deployment (`docker compose build` / `up --build`) from the
standalone `mm-chat/` tree. Registry-published immutable image digests remain an
optional hardening/promotion path through `scripts/release-images.sh`, but GHCR
push or digest-env proof is no longer required to prove `mm-chat/` is an
independent project.

Slice sequence:

- [x] G10.1 Former-root delete-plan dry run: add a non-destructive candidate
      manifest script, protected-path boundary, approval phrase, rollback
      steps, and deployment-doc index link.
- [x] G10.2 Operations and backup/restore closure: record backup checksums,
      Postgres temporary restore drill, MinIO restore drill, Compose
      source-build/restart/rollback evidence, and runtime smoke.
  - [x] G10.2a Local live-stack backup/restore smoke: using a temporary
        `BACKUP_DIR`, Postgres and MinIO backups, checksum verification,
        disposable Postgres restore, temporary MinIO bucket restore, Compose
        config, restart, and readiness smoke passed; temp backup artifacts and
        restore DB were removed after verification.
  - [x] G10.2b Build-based Compose closure: from `mm-chat/`, Compose built
        backend, frontend, and RAG worker images from local source, ran
        migrations, recreated backend/frontend/rag-worker, and passed frontend,
        `/mm-api/ready`, backend `/ready`, and RAG worker health smokes.
    - [x] G10.2b.1 Optional release image script: `scripts/release-images.sh`
          still builds backend, frontend, and RAG images, supports local
          `--load`, explicit `--push`, and immutable `@sha256:` env emission
          for future registry promotion, but it is not a required standalone
          deletion gate.
    - [x] G10.2b.2 Compose source-build proof: `docker compose ... build
          backend frontend rag-worker` and `docker compose ... up -d backend
          frontend rag-worker` passed using only the standalone project tree.
- [x] G10.3 Visual/interaction closure: record desktop and mobile smoke for
      app shell, chat streaming, model/provider visibility, Knowledge citation
      cards, Files/upload when configured, and navigation.
  - [x] G10.3a Automated UI/visual contract smoke: frontend tests for app
        shell/mobile accessibility, citation styling/cards, markdown rendering,
        server Knowledge wiring, model resolution, and server defaults passed;
        HTTP app shell returned valid Next HTML.
  - [x] G10.3b Browser screenshot/interaction smoke: Windows Chrome CDP smoke
        captured desktop and mobile screenshots, verified the app shell,
        composer placeholder, model/provider control visibility, Knowledge
        citation card, Knowledge page/drawer, and search-toggle fail-closed
        toast.
- [ ] G10.4 Owner-confirmed former-root cleanup: after all gates pass, require
      the exact owner approval phrase before running any generated destructive
      command.

Targeted tests:

- final full frontend, Go, RAG, Compose, security, backup/restore,
  visual-regression, and clean-copy gates;
- deletion dry-run before any destructive action.

## G11 — Owner Parity Regression Closure

Owner live testing reopened three parity gaps after the standalone source-build
proof: image understanding, accidental Team UX, and browser-configured provider
settings. This group supersedes the prior multi-user UI assumption for the
current local standalone target: `mm-chat/` returns to the original single-user
experience while keeping Go backend, Python RAG worker, and Postgres as runtime
services.

Slice sequence:

- [x] G11.1 Chat image understanding: server-mode uploaded `image/*` message
      attachments are read back from private file storage, forwarded to the
      configured OpenAI-compatible provider as multimodal `image_url` data URL
      parts, and covered by handler/provider tests.
- [x] G11.2 Single-user Team removal: deleted the frontend Team settings path,
      deep-link tab, locale bundle, and composition test; Knowledge collection
      creation now stays Personal-only in the standalone UI. Backend Team code
      remains only as unreachable legacy surface until a later schema/API
      cleanup slice can safely drop migrations and packages.
- [ ] G11.3 Browser provider settings parity: restore original-style web
      provider add/edit/model-fetch flow for local single-user deployments and
      make Go chat/model calls consume the selected browser-configured provider
      without logging or documenting secrets.

Targeted tests:

- `cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...`;
- focused frontend composition/type/lint tests for Team removal and provider
  settings;
- live local chat smoke with image input and selected provider once G11.3 lands.

## Completion Ledger

| Group                                    | Status      | Completion Rule                                                                      |
| ---------------------------------------- | ----------- | ------------------------------------------------------------------------------------ |
| G0 Plan Freeze and Guardrails            | Complete    | Docs, indexes, progress, and process log updated                                     |
| G1 Conversation and Message Operations   | Complete    | G1.1-G1.6 complete; only paused cross-group search toggle remains outside G1         |
| G2 Related Questions and Agent Catalogs  | Complete    | Related-question/catalog Next routes replaced                                        |
| G3 Auth, Config, Provider Settings, BYOK | Complete    | Server-auth/config/provider lifecycle verified                                       |
| G4 Plugin Final Ownership                | Complete    | G4.5c/G4.6b Go ownership and G9.4 Next route deletion complete                       |
| G5 Search/Web Enrichment                 | Paused      | Owner reopens, then server-owned search passes gates                                 |
| G6 Voice/Image/Code Jobs                 | In progress | Image generation is reopened through Go artifacts; voice executor remains            |
| G7 Knowledge/RAG/Citations               | Complete    | Live MinerU + Jina + Postgres strict citation loop passed                            |
| G8 Teams/Knowledge UI                    | Complete    | G8.1-G8.5 frontend control-plane wiring and isolation smoke passed                   |
| G9 Data Authority/Route Removal          | Complete    | G9.1-G9.6 route freeze, route deletion, local write-authority, and clean-copy preflight passed |
| G10 Final Closure/Delete Plan            | In progress | G10.1-G10.3 and build-based G10.2 complete; owner cleanup blocked by G11 parity regressions |
| G11 Owner Parity Regression Closure      | In progress | G11.1 image understanding and G11.2 Team UI removal complete; browser provider settings remain |

## Update Discipline

When a group starts or finishes:

1. update this ledger if the group state changes;
2. update `../tracking/progress.md` group checkboxes;
3. append evidence to
   `../tracking/standalone-parity-sliced-process.md`;
4. only append to the legacy `../tracking/process.md` for major milestones or
   cross-reference notes;
5. keep secrets, provider keys, user data, and private logs out of docs.
