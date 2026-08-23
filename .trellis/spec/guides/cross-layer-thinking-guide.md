# Cross-Layer Thinking Guide

> **Purpose**: Think through data flow across layers before implementing.

---

## The Problem

**Most bugs happen at layer boundaries**, not within layers.

Common cross-layer bugs:
- API returns format A, frontend expects format B
- Database stores X, service transforms to Y, but loses data
- Multiple layers implement the same logic differently

---

## Before Implementing Cross-Layer Features

### Step 1: Map the Data Flow

Draw out how data moves:

```
Source → Transform → Store → Retrieve → Transform → Display
```

For each arrow, ask:
- What format is the data in?
- What could go wrong?
- Who is responsible for validation?

### Step 2: Identify Boundaries

| Boundary | Common Issues |
|----------|---------------|
| API ↔ Service | Type mismatches, missing fields |
| Service ↔ Database | Format conversions, null handling |
| Backend ↔ Frontend | Serialization, date formats |
| Component ↔ Component | Props shape changes |

### Step 3: Define Contracts

For each boundary:
- What is the exact input format?
- What is the exact output format?
- What errors can occur?

---

## Common Cross-Layer Mistakes

### Mistake 1: Implicit Format Assumptions

**Bad**: Assuming date format without checking

**Good**: Explicit format conversion at boundaries

### Mistake 2: Scattered Validation

**Bad**: Validating the same thing in multiple layers

**Good**: Validate once at the entry point

### Mistake 3: Leaky Abstractions

**Bad**: Component knows about database schema

**Good**: Each layer only knows its neighbors

---

## Checklist for Cross-Layer Features

Before implementation:
- [ ] Mapped the complete data flow
- [ ] Identified all layer boundaries
- [ ] Defined format at each boundary
- [ ] Decided where validation happens

After implementation:
- [ ] Tested with edge cases (null, empty, invalid)
- [ ] Verified error handling at each boundary
- [ ] Checked data survives round-trip

## Producer / Validator Bound Checklist

Use this when one layer produces a bounded numeric value and another layer
rejects values outside that bound:

- [ ] Define the minimum/maximum once and import them in every producer and
      validator; duplicated literals are contract drift.
- [ ] Test a value just below the minimum, exactly at both bounds, and just
      above the maximum through the complete producer-to-validator path.
- [ ] If the producer cannot meet a quality minimum without breaking source
      lineage or a hard maximum, represent the documented absence/fallback;
      do not emit a value the next layer must reject.
- [ ] Track the value for the state actually selected. A later rejected
      candidate must not overwrite the selected-state counter used at return.

**Real-world example**: The RAG Chunk planner selected a 58-token exact overlap,
then inspected an earlier atom that pushed the candidate above 100. The
rejected candidate count masked the selected count, so the planner emitted 58
while both Native and MinerU mappers required 60–100 and rejected the complete
document.

## Live Stream vs Durable Replay Checklist

Use this when the same operation is shown first from SSE/WebSocket events and
later from persisted history:

- [ ] Capture the actively served live event shape before changing either
      producer or consumer; checked-in DTOs and comments are secondary.
- [ ] Compare live and durable identities, optional/redacted fields, enums, and
      classification values field by field. Do not assume both paths serialize
      the same projection.
- [ ] Keep the live client boundary typed as untrusted data and normalize once.
      If a field is absent by contract, add a narrow fallback; a present
      malformed field must still fail closed.
- [ ] Test the actual stream dispatch before reload, then test persisted replay
      separately. A successful reload can hide a broken live parser because
      detached Backend execution may finish and persist after the UI aborts.
- [ ] Trace every client-side conversion after normalization. In particular,
      compare the live draft, terminal Message replacement, store-domain
      conversion, and list/reload conversion field by field; a typed DTO field
      can still disappear in a handwritten object mapper without a compiler
      error.
- [ ] Assert the terminal replacement and a fresh reload retain the same durable
      event collection. Do not stop at proving that details appear while SSE is
      still streaming.
- [ ] For rollout acceptance, retain one complete pre-reload stream trace and
      prove the final UI state without using reload as recovery.

**Real-world example**: Backend `local_direct` events redacted Provider
`callId`, retained `executionId`, and emitted File mutations as `write`.
Frontend required `callId` and allowed only `read|execute`, so the browser
reported an invalid Tool update while Backend execution continued. Reload later
rendered the durable timeline and made an API-only acceptance look healthy.

## Applied Migration Immutability Checklist

Use this before editing any committed migration or changing the repository
schema head:

- [ ] Determine whether either SQL direction has been applied to any retained
      environment. Treat comments, whitespace, line endings, and terminal
      blank lines as checksum-bearing bytes.
- [ ] Compare the embedded migration checksum with the persistent
      `schema_migrations` ledger before changing source. Runtime function shape
      does not override ledger evidence because a later repair may already have
      replaced the function body.
- [ ] If an applied pair drifted, restore its exact applied bytes, pin the live
      checksum in a regression test, and carry every behavioral correction in
      a new forward migration. Never edit `schema_migrations.checksum` to bless
      changed source.
- [ ] Reassert function owner-sensitive security properties in the forward
      repair: `SECURITY DEFINER`, safe `search_path`, revokes, and exact grants.
- [ ] Update current-head assertions and historical rollback drills together.
      A drill targeting an older boundary must explicitly defer a later
      irreversible migration, remove only its synthetic ledger rows before
      down, and replay the real tail afterward.
- [ ] Prove fresh install, exact retained-ledger upgrade, repeated up, and safe
      down/re-up against the supported PostgreSQL major before deployment.

**Real-world example**: A Chat Agent gateway fix directly edited migration
`096` after production had recorded its checksum. The runtime function was
correct, but the next deployment stopped before `098`. The recovery restored
the original `096` bytes, pinned its applied checksum, and moved the idempotent
function repair into forward-only migration `099`.

## Long-Running Provider Response Checklist

Use this before attributing a slow AI/media request to a reverse proxy:

### Before changing timeouts

- [ ] Trace every deadline in order: browser/client → application handler →
      provider HTTP client → reverse proxy → upstream service
- [ ] Distinguish status ownership: a proxy `499` means its client disconnected;
      it is not proof of an upstream read timeout such as `504`
- [ ] Verify the configuration loaded by the active process, not only a file on
      disk; include config validation and worker start/reload evidence
- [ ] Check the provider's official async/SSE contract before increasing a
      synchronous deadline

### When introducing provider SSE

- [ ] Record time to response headers, first event, final event, and EOF
- [ ] Test both image-bearing completion and empty completion-marker shapes;
      retain the last valid partial as the final fallback
- [ ] Bound the total response and decoded artifact sizes, sanitize malformed
      stream failures, and never put provider payloads or credentials in errors
- [ ] Keep partial artifacts ephemeral unless the product explicitly exposes
      previews; persist only the final artifact
- [ ] Run the real end-to-end path from the deployed caller and delete the
      generated file/conversation fixture afterward

Canonical project example:
`mm-chat/docs/contracts/media-job-executor-seams.md` defines the executable
GPT Image request, response, validation, and test contract.

## Host Shell vs Deployed Egress Checklist

Use this when a developer command can read a public URL but an application
Tool, worker, or container cannot:

- [ ] Record whether the host command inherited `HTTP_PROXY`, `HTTPS_PROXY`, or
      `ALL_PROXY`; repeat the proof with ambient proxies disabled.
- [ ] Run DNS and connection checks from the actual deployed caller/network.
      A public-shaped DNS answer is not proof that the route is usable.
- [ ] Do not enable an ambient proxy inside a DNS-pinned SSRF client merely to
      match host-shell behavior. Proxy-side DNS can invalidate the checked-IP
      binding.
- [ ] Keep keyword Search, exact URL extraction, and browser automation as
      separate capabilities. Prefer the exact selected Provider's bounded
      Extract API for restricted public egress, then a no-proxy safe reader;
      never silently escalate to browser authority.
- [ ] Validate the user URL locally before any provider Extract call and keep
      model arguments, redirects, source bodies, and provider errors out of
      durable diagnostics.
- [ ] Verify one real URL through the deployed application path after rollout,
      then confirm private/literal targets still fail before outbound work.

Canonical project example:
`mm-chat/docs/contracts/go-web-search-providers.md` owns `read_web_url`, Tavily
Extract, Discourse JSON adaptation, and the `safenet` direct fallback.

## Semantic Provider Capability Checklist

Use this when one UI control maps to different provider request contracts:

- [ ] Store a provider-neutral semantic value in product state; translate it at
      the backend boundary instead of exposing one provider's raw budget/enum
- [ ] Decide and test both persistence scopes: current server-owned entity and
      the browser default used when a new entity is created
- [ ] Preserve legacy records that predate the new field with an explicit
      compatibility fallback
- [ ] Validate the browser value in Go and clamp model-specific extensions;
      never forward an arbitrary string to the provider
- [ ] Keep Auto distinct from Off: Auto uses the provider/model default, while
      Off disables explicit capability activation
- [ ] Unit-test every provider mapping and run a deployed selection/reload proof
      so an in-memory UI update cannot masquerade as durable persistence

Canonical project example:
`mm-chat/docs/contracts/chat-stream-api.md` owns the reasoning effort request,
normalization, provider mapping, and backward-compatibility contract.

## Shared UI Projection vs Entity Authority Checklist

Use this when one selector/control is visible globally but its value belongs to
the currently selected persisted entity:

- [ ] Name the durable owner first (for example `Conversation.modelRef`) and
      treat global Zustand state only as the active UI/runtime projection.
- [ ] Trace both writes and restores: selector → entity persistence → list/read
      DTO → entity mapper → selection action → active projection.
- [ ] Keep the browser preference separate and define exactly when it seeds a
      new/legacy entity; selecting an existing entity must not rewrite it.
- [ ] Test two entities with different values, switch A → B → A, refresh, and
      assert that a write to A never mutates B.
- [ ] Serialize same-entity writes or reject stale completions so rapid changes
      cannot make an older request durable last.
- [ ] If backend and frontend identities use aliases, normalize at the entity
      boundary and test the exact available-catalog match.

The executable frontend model-selection contract lives in
`../frontend/state-management.md` under “Conversation-owned model selection.”

## Authentication Mode vs Fixed-Owner Bootstrap Checklist

Use this when server startup creates governance, consent, or other runtime
authority for a single-server owner:

- [ ] Separate request authentication from bootstrap scope. `required` may
      require a real Session while still using one fixed bootstrap owner for
      server-managed setup.
- [ ] Trace the exact owner ID through Provider configuration, governance,
      query consent, collection consent, and runtime authorization.
- [ ] Preserve Provider identity across layers. A shared wire protocol does not
      make `server-default` and a server-stored Provider the same endpoint.
- [ ] Cover both existing-state backfill and future-resource creation. Scope
      automatic creation hooks to the fixed owner so invited users do not
      inherit owner authority.
- [ ] Test every supported auth mode around startup wiring; package-level
      consent tests alone cannot detect a mode guard in `cmd/api`.
- [ ] For live diagnostics, resolve the actual container image, environment,
      network, and mounted keyring path first. A stale local keyring can create
      a false Provider-unavailable result even when production is healthy.
- [ ] Persist only a fixed failure-stage enum. Keep queries, source bodies,
      credentials, URLs, and raw errors outside durable diagnostics.

**Real-world example**: Knowledge worked through retrieval and hydration, but
required-auth startup skipped answer-consent provisioning. Chat could use the
attested `PJRSVY` Provider while Knowledge correctly denied its distinct
`pjrsvy/server-stored/<model>` answer egress. The repair retained exact
Provider identity and bootstrapped only the fixed owner in both auth modes.


## Contextual Media Routing and Raw HTML Checklist

- [ ] Test follow-up language against bounded active-branch history, not only
      the current prompt; require a recent media-generation artifact/model.
- [ ] Keep analysis/negation phrases on the chat path and preserve attachment
      restrictions when the media executor is prompt-only.
- [ ] Remember that CommonMark raw HTML blocks can terminate at internal blank
      lines even when every HTML tag is balanced.
- [ ] Keep incomplete streaming visual tails inert until their root fragment is
      balanced; do not let partial CSS mutate the conversation layout.
- [ ] Never broaden inline CSS merely to make one generated layout work. Keep
      flow-layout HTML inline; move positioned/transformed layouts to the
      nonce-constrained inline-sandbox boundary, without `allow-same-origin`.
- [ ] Fit fixed poster canvases against both iframe width and height; a sandbox
      boundary alone does not make absolute pixel coordinates responsive.
- [ ] Re-test an actual persisted failing message after deployment so a small
      synthetic fragment cannot hide parser-boundary differences.

Canonical project example:
`mm-chat/docs/contracts/frontend-api-client.md` owns contextual image
continuation and HTML visual fallback behavior.

---

## Cross-Platform Template Consistency

In Trellis, command templates (e.g., `record-session.md`) exist in **multiple platforms** with identical or near-identical content. This is a cross-layer boundary.

### Checklist: After Modifying Any Command Template

- [ ] Find all platforms with the same command: `find src/templates/*/commands/trellis/ -name "<command>.*"`
- [ ] Update all platform copies (Markdown `.md` and TOML `.toml`)
- [ ] For Gemini TOML: adapt line continuations (`\\` vs `\`) and triple-quoted strings
- [ ] Run `/trellis:check-cross-layer` to verify nothing was missed

**Real-world example**: Updated `record-session.md` in Claude to use `--mode record`, but forgot iFlow, Kilo, OpenCode, and Gemini — caught by cross-layer check.

---

## Generated Runtime Template Upgrade Consistency

Some generated files are both documentation and runtime input. In Trellis,
`.trellis/workflow.md` is parsed by `get_context.py`, `workflow_phase.py`,
SessionStart filters, and per-turn hooks. Template changes must be validated
against both fresh init and upgrade paths.

### Checklist: After Modifying A Runtime-Parsed Template

- [ ] Identify every runtime parser that reads the template, not just the file
  writer that installs it
- [ ] Check whether relevant syntax lives outside obvious managed regions
  such as tag blocks
- [ ] Verify fresh `init` output and a versioned `update` scenario that writes
  the older `.trellis/.version`
- [ ] Add an upgrade regression using an older pristine template fixture, then
  assert the installed file reaches the current packaged shape
- [ ] Update the backend spec that owns the runtime contract

**Real-world example**: Codex inline mode changed workflow platform markers from
`[Codex]` / `[Kilo, Antigravity, Windsurf]` to `[codex-sub-agent]` /
`[codex-inline, Kilo, Antigravity, Windsurf]`. Fresh init was correct, but
`trellis update` only merged `[workflow-state:*]` blocks and preserved stale
markers outside those blocks. Result: upgraded projects got new hook scripts
but old workflow routing, so `get_context.py --mode phase --platform codex`
could return empty Phase 2.1 detail.

---

## Mode-Detection Probe Checklist

When a CLI auto-detects a mode by probing a remote resource (e.g., checking if `index.json` exists to decide marketplace vs direct download):

### Before implementing:
- [ ] Probe runs in **ALL** code paths that use the result (interactive, `-y`, `--flag` combos)
- [ ] 404 vs transient error are distinguished — don't treat both as "not found"
- [ ] Transient errors **abort or retry**, never silently switch modes
- [ ] Shared state (caches, prefetched data) is **reset** when context changes (e.g., user switches source)
- [ ] **Shortcut paths** (e.g., `--template` skipping picker) must have the same error-handling quality as the probed path — check that downstream functions don't call catch-all wrappers

### After implementing:
- [ ] Trace every path from probe result to the mode-decision branch — no fallthrough
- [ ] External format contracts (giget URI, raw URLs) are tested or at least documented as comments
- [ ] Metadata reads consume a complete response or use a streaming parser — never parse a fixed-size prefix as full JSON
- [ ] When reconstructing a composite identifier from parsed parts, verify **all** fields are included and in the **correct position** (e.g., `provider:repo/path#ref` not `provider:repo#ref/path`)
- [ ] Verify that **action functions** called after a shortcut don't internally use the old catch-all fetch — they must use the probe-quality variant when error distinction matters

**Real-world example**: Custom registry flow had 8 bugs across 3 review rounds: (1) probe only ran in interactive mode, (2) transient errors fell through to wrong mode, (3) giget URI had `#ref` in wrong position, (4) prefetched templates leaked across source switches, (5) `--template` shortcut bypassed probe but `downloadTemplateById` internally used catch-all `fetchTemplateIndex`, turning timeouts into "Template not found".

**Real-world example**: Agent-session update hints fetched npm `latest` metadata with `response.read(4096)` and then parsed it as complete JSON. The `@mindfoldhq/trellis` package metadata exceeded 4 KB, so the JSON was truncated, parse failed silently, and the first session injection showed no update hint. Fix: read the complete response before parsing, and add a regression where `version` is followed by an 8 KB metadata tail.

---

## When to Create Flow Documentation

Create detailed flow docs when:
- Feature spans 3+ layers
- Multiple teams are involved
- Data format is complex
- Feature has caused bugs before
