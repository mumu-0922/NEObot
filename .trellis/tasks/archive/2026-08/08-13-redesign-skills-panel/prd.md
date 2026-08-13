# Phase 1: My Assistants and Assistant Store

## Goal

Phase 1 upgrades the existing Assistant catalog into a true `My Assistants |
Assistant Store` lifecycle using lightweight LobeHub-compatible prompt presets.
After that foundation is shipped and manually verified, a separate follow-up
will redesign Skills so the three product concepts remain distinct.

## What I Already Know

- The current top-level `SkillMarket` mixes installed Skills, the entire
  built-in catalog, search, category filters, pagination, creation, editing,
  installation, and removal in one long page.
- The composer Sparkles menu selects installed Skills for the current
  Conversation, but only shows titles; it does not explain whether selection
  means eligible-for-auto-routing or always-applied guidance.
- Installed Skills and custom Skills are browser-owned in `settingsStore`.
- Skills are pure text. Normalization rejects scripts, external Tools, network,
  and non-text Skills.
- Server-mode chat currently applies only explicitly selected Skills and calls
  `resolveSkillsForMessage(... autoSelect: false)`; local-mode branches may use
  the global `skillAutoSelect` setting.
- A used Skill is already shown above an assistant answer as a small chip.
- The current catalog has many narrow writing, analysis, product, learning,
  safety, support, and developer Skills, so the single-page card wall becomes
  visually noisy.
- LobeHub Agent entries are primarily reusable Agent presets. Their core is a
  system prompt defining identity, expertise, behavior, and workflow; richer
  entries may also declare chat settings and Tool/Plugin dependencies. Neo Chat
  currently imports only the bounded display metadata and `systemRole`, so in
  this product they execute as Conversation-level prompt presets rather than a
  richer independent Agent runtime.

## Assumptions (Temporary)

- Preserve the existing pure-text execution model and current Skill catalog;
  this task initially targets information architecture and control semantics,
  not MCP-like code execution.
- Keep Skills separate from Assistants and Tools instead of merging the three.
- Prefer a management page plus a compact Conversation picker, matching the
  product's existing Tools pattern without copying its protocol terminology.

## Research Notes

Assistant-store feasibility was also checked because it changes the product
navigation and the perceived boundary between Assistants and Skills. See
[`research/assistant-market-fit.md`](research/assistant-market-fit.md). The
current Assistant catalog already adapts the legacy LobeHub Agent registry, but
uses selection/recent-use semantics rather than explicit installation.

### Approach A — Installed / Discover / Create (Recommended)

- Top-level tabs separate daily management from browsing.
- `Installed` shows enabled state, short purpose, applicability, and edit/remove.
- `Discover` owns search, categories, catalog cards, detail, and install.
- `Create` is a focused editor flow rather than a tiny action hidden among cards.
- Composer remains a compact selector with a clear `Auto` or `Always use`
  semantic per selected Skill.

Pros: lowest clutter, familiar after the Tools redesign, preserves existing
data model, and scales to a larger catalog. Cons: requires one extra tab click
when browsing.

### Approach B — Task-first library

- Organize the page around user goals such as Write, Analyze, Learn, and Build.
- Recommend a few Skills before showing the full catalog.

Pros: friendlier for nontechnical users. Cons: category quality becomes product
authority and can hide installed/active state.

### Approach C — Merge Skills into Assistants

- Make Skills capabilities configured inside an Assistant.

Pros: fewer top-level concepts. Cons: breaks reusable per-conversation Skill
selection and falsely implies that a Skill owns persona/model/tool settings.
This is not recommended.

## Requirements (Evolving)

- Phase 1 is the Assistant area only. Skills behavior and UI remain unchanged
  in this implementation slice.
- Replace the current mixed/recent-use Assistant hub with `My Assistants |
  Assistant Store` tabs.
- Installing a Store Assistant adds it to My Assistants without immediately
  creating or mutating a Conversation.
- Starting an Assistant from My Assistants creates a new Conversation with its
  bounded System Prompt; an untouched empty Conversation may still be reused
  if that behavior remains safe and unsurprising.
- Store cards open a detail surface. Installation and starting a Conversation
  are separate explicit actions.
- A Store Assistant must be installed before it can start a Conversation.
  There is no one-off Try state in Phase 1.
- Uninstalling an Assistant removes only its installed-library entry. Existing
  Conversations and their copied System Prompt remain unchanged and are never
  deleted or rewritten by uninstall.
- Persist installed Assistants per authenticated user in Backend storage so
  they survive browser cache clearing and device changes. Preserve custom
  Assistant authoring for newly created entries, but do not migrate any current
  browser-owned custom, used, cached, or overridden Assistant state.
- Phase 1 supports creating, editing, and deleting server-owned custom
  Assistants. Custom and Store-installed Assistants share My Assistants but keep
  distinct source badges and lifecycle rules.
- Deleting a custom Assistant requires an explicit confirmation dialog and is
  permanent for the library entry. It never deletes or rewrites Conversations
  previously created from that Assistant.
- Custom Assistant updates use a server-issued revision and compare-and-swap
  semantics. If another device saved a newer revision first, the stale save is
  rejected instead of overwriting it; the editor preserves the user's draft
  and offers reload/copy recovery.
- Phase 1 custom Assistant fields are limited to avatar, title, description,
  category, tags, and bounded System Prompt. Opening messages, suggested
  questions, model defaults, and Tool bindings are deferred.
- My Assistants uses one deterministic order: most recently installed or
  modified first, with stable identity as the tie-breaker. Phase 1 has no
  favorites, pinning, or drag ordering.
- Store-installed Assistants are immutable snapshots in My Assistants. Editing
  one creates a separate server-owned custom copy with a new identity; it never
  mutates the installed Store artifact or creates an update conflict.
- Store updates are explicit and manual. The UI may report that a newer
  upstream snapshot exists, but replaces the installed snapshot only after the
  user confirms. Existing Conversations and custom copies remain unchanged.
- Update identity uses a Backend-generated canonical content fingerprint over
  the supported normalized Assistant snapshot. The legacy Registry exposes no
  semantic version or reliable modification time, so `createdAt` is never used
  as update authority. Store listing may compare a bounded summary fingerprint;
  installation/update fetches detail and stores the exact detail fingerprint.
- Every Store detail and install confirmation displays a fixed compatibility
  notice: installation includes only the role and prompt, not any plugin,
  Knowledge Base, external Tool, or capability mentioned inside that prompt.
  Structured dependencies, when trustworthy and supported, may be shown in
  addition; Phase 1 never guesses dependencies by scanning prompt text.
- Reuse the established MCP Marketplace information architecture: top-level
  `My Assistants | Assistant Store` tabs, store search and category rail,
  paginated/infinite catalog results, authoritative installed state, and a
  dedicated detail/install surface. Preserve Assistant-specific avatar,
  description, author, prompt-preview, and start-chat presentation; do not copy
  MCP protocol or connection terminology.
- On entry, open My Assistants when the current user has at least one installed
  or custom Assistant; otherwise open Assistant Store. Do not persist the last
  tab as a competing browser authority.
- Assistant Store browsing uses server-authoritative pages of 20 with bounded
  infinite scrolling. A reset search/category change replaces page 1 and aborts
  or generation-fences stale work; near-bottom appends exactly one next page
  with identifier deduplication. Next-page failure preserves loaded cards and
  exposes manual load/retry.
- Store discovery prefers a bounded Backend adapter over LobeHub's current
  public React Router `.data` route and falls back to the legacy Agent Registry
  when the live adapter is unavailable or its schema drifts. The browser never
  fetches or parses upstream HTML/data directly. Upstream global market count
  and browseable result count remain distinct.
- Installation is fail-closed on Store detail authority. If the bounded System
  Prompt is missing, invalid, unavailable, or its normalized fingerprint does
  not match the confirmed detail, create no installed row and show a scoped
  retryable detail/install error. Never synthesize a replacement prompt from
  title or description and never persist a partial unusable Assistant.
- Ordinary users may install only the Neo Chat admitted subset of live-market
  Assistants. Unreviewed, jailbreak, dangerous financial, privilege-seeking,
  or otherwise disallowed upstream entries are hidden or non-installable;
  upstream publication alone never grants install authority. Administrator
  review/admission remains a separate authority boundary.
- Explain Skills in plain language as reusable text workflows/instructions.
- Do not describe Skills as Tools, MCP servers, plugins, or executable code.
- Reduce the current one-page mixture of installed management and catalog
  browsing.
- Preserve current custom Skill creation/editing and built-in Skill install.
- Preserve visible per-answer Skill usage evidence.
- Treat LobeHub Assistants as lightweight Conversation presets: import bounded
  display metadata, System Prompt, source/version provenance, and display-only
  Tool dependency declarations. Never install or enable MCP, provider secrets,
  model settings, or permissions implicitly with an Assistant.

## Acceptance Criteria (Evolving)

- [ ] Store browsing, installation, My Assistants, start-chat, uninstall, and
      installed-state round-trip through server-authoritative APIs.
- [ ] Clicking a Store card cannot silently create a Conversation.
- [ ] Installing an Assistant cannot install/enable MCP, change the selected
      model/provider, or import secrets.
- [ ] Missing declared Tool dependencies are visible before starting and link
      to Tools without granting authority.
- [ ] First load of the new system starts My Assistants empty regardless of
      legacy browser-owned Assistant state; old data grants no installation.
- [ ] A user can create, edit, start, and delete a custom Assistant from My
      Assistants on another device after signing into the same account.
- [ ] Deleting a custom Assistant requires confirmation; cancellation changes
      nothing, while confirmation removes only the library entry and preserves
      all existing Conversations.
- [ ] Concurrent custom-Assistant edits cannot silently overwrite each other:
      a stale revision receives a conflict response, the newer server value is
      preserved, and the rejected editor draft remains recoverable.
- [ ] Editing a Store Assistant creates a custom copy; subsequent Store updates
      or uninstall do not modify or remove that custom copy.
- [ ] An available Store update is visible and requires explicit confirmation;
      no background refresh changes the installed System Prompt.
- [ ] Every Store install surface warns that prompt-mentioned plugins,
      Knowledge Bases, and external Tools are not included; no heuristic prompt
      scan grants or claims a dependency.
- [ ] Ordinary-user Store results and install APIs enforce the same Neo Chat
      admission decision server-side; changing client requests cannot install
      an unadmitted upstream Assistant.
- [ ] A user can tell the difference between Skill, Assistant, and Tool from the
      Skills page without opening documentation.
- [ ] Installed Skills and discoverable Skills are not interleaved in one card
      wall.
- [ ] Conversation Skill selection communicates its actual routing/application
      behavior.
- [ ] Custom creation remains available but does not dominate the main page.
- [ ] Existing Skill storage and pure-text safety boundaries remain intact.

## Definition of Done

- Focused Backend tests cover ownership, admission enforcement, install/update
  fingerprint checks, optimistic concurrency, deletion, and Conversation
  snapshot preservation.
- Focused Frontend tests cover tabs, store paging, installed state,
  empty/loading/error states, create/edit/delete, conflicts, and start-chat.
- Backend `go test` for affected packages plus Frontend formatting, lint,
  typecheck, and focused Vitest pass; no unnecessary full-suite run.
- Assistant API, persistence, security, and deployment documentation are
  updated where behavior changes.
- Runtime frontend is rebuilt and deployed for manual verification.

## Open Questions

- None for the Phase 1 Assistant implementation. Skill selection semantics are
  intentionally deferred to the separate Skills redesign.

## Out of Scope (Initial)

- Phase 1 does not redesign the Skills page or composer Skill picker.
- Executable Skills, scripts, network access, MCP calls, or file access.
- Merging Assistants, Skills, and Tools into one entity.
- Mirroring or bulk-ingesting the full LobeHub market.
- Automatic Assistant moderation based only on prompt keyword scanning.
- Importing Assistant-declared models, secrets, Knowledge Bases, MCP servers,
  plugins, permissions, or other executable capabilities.

## Decision (ADR-lite, Evolving)

**Context:** LobeHub Agent entries are primarily prompt presets, but richer
entries may declare model settings and Tool/Plugin dependencies that Neo Chat
must not silently import.

**Decision:** Use the lightweight Assistant-package model. Installation saves
only a bounded role preset and provenance; missing Tool dependencies are shown
and linked to the existing Tools marketplace but remain separately installed
and enabled. Store Assistants must be installed before use; Phase 1 has no
one-off Try lifecycle. Uninstall never mutates or deletes existing
Conversations because each Conversation owns the prompt snapshot applied when
it was created. Legacy browser-owned Assistant state is not migrated into the
new server-owned library.

**Consequences:** Assistant installation stays reversible and understandable,
while full-fidelity LobeHub Agent runtime compatibility remains out of scope.

## Technical Approach

- Add server-owned Assistant tables for custom and Store-installed snapshots,
  including user ownership, source identity, normalized content fingerprint,
  revision, provenance, timestamps, and soft authority metadata where needed.
- Expose authenticated Assistant library CRUD, Store browse/detail,
  install/update/uninstall, and start-chat-compatible APIs. Enforce ownership,
  admission, validation, and optimistic revision checks in Backend handlers and
  services rather than trusting UI state.
- Keep Store discovery behind a bounded Backend adapter. Validate the live
  LobeHub `.data` response against an explicit schema, timeout/size/page limits,
  and cache policy; fall back to the legacy Registry without claiming the live
  global count as browseable inventory.
- Keep admission as explicit server-side authority, not a prompt-text heuristic:
  ordinary-user browse/install responses include only admitted identities;
  administrators can review upstream entries and change admission state.
- Replace the recent-use Assistant hub with `My Assistants | Assistant Store`,
  using the established Marketplace UX patterns while keeping Assistant-only
  language and surfaces.
- Create a Conversation from a validated Assistant snapshot, copying its System
  Prompt so later update, uninstall, or deletion cannot mutate history.

## Implementation Plan

1. Add migration, domain model, repository/service contracts, admission and
   revision rules, plus focused Backend tests.
2. Add bounded live/fallback Store adapters and authenticated Assistant APIs,
   including fail-closed detail installation and focused handler tests.
3. Build `My Assistants | Assistant Store`, detail/install/update flows, custom
   editor/delete/conflict recovery, and focused Frontend tests.
4. Wire start-chat snapshot behavior, update documentation, run only affected
   quality gates, rebuild/deploy, and perform a manual smoke test.

## Technical Notes

- Page: `mm-chat/frontend/src/components/skill/SkillMarket.tsx`
- Composer picker: `mm-chat/frontend/src/components/chat/MessageInput.tsx`
- Runtime resolution: `mm-chat/frontend/src/services/api/skillService.ts`
- Normalization/context: `mm-chat/frontend/src/lib/skills/index.ts`
- State: `mm-chat/frontend/src/store/core/settingsStore.ts`
- Applied marker: `mm-chat/frontend/src/components/chat/MessageItem.tsx`
