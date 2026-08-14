# Agent Runtime G20.9 — Legacy Skill deletion and held Runtime cutover

## Goal

Hard-delete the legacy browser pure-text Skill state, catalog, editor,
selection and prompt-execution chain without migration or package matching;
preserve old messages only as the read-only fact “旧版技能已退役”; retain package
Skill/Agent Center controls as the sole eligible Runtime domain while exact-host
execution remains honestly held.

## Shared baseline

- The user approved all recommended decisions, direct commits and deletion
  choice A: old pure-text Skill data is deleted and packages may be installed
  later from the Store or another explicit source.
- No additional preference prompt or sub-Agent is allowed or needed.
- G20.1-G20.8 contracts remain binding. Package Skills, Assistants and MCP Tools
  are separate identity/authority domains.
- Current host status is `ISOLATION_UNAVAILABLE`. Source cutover must not invent
  executable production evidence or use API/browser/rootful fallback.
- G20.8 is the pre-cutover inventory/backup/dry-run release. Production G20.9
  promotion requires its verified backup/fingerprint plus database backup/count.

## Requirements

### Hard-delete browser authority

- Advance browser persistence once and delete the eight legacy settings fields
  from both raw top-level records and nested Zustand `state` envelopes:
  `installedSkills`, `customSkills`, `activeSkillIds`, `skillAutoSelect`,
  `skillCatalogs`, `skillCatalogTimestamps`, `skillDefinitions`, and
  `skillDefinitionTimestamps`.
- Remove Session `config.activeSkills` and Workspace `activeSkills` from types,
  stores, normalizers, imports/exports and persisted records.
- Settings and Chat `partialize` must never write a retired field again. Raw
  migration completion is marked only after all affected browser records
  succeed; retry/reload is idempotent and cannot resurrect state.
- Preserve Assistant, MCP, Chat content/tree, files/attachments, Knowledge,
  Memory, provider, voice, search and core preference values. Do not upload or
  log raw legacy bodies.
- Keep production deletion backup-gated: G20.8 raw backup and manifest
  fingerprint precede deployment; an explicit PostgreSQL preflight/apply path
  removes only Conversation metadata `activeSkills` after backup and expected
  count confirmation.

### Delete legacy product and execution code

- Delete the legacy Skill editor/panel, sidebar entry, URL panel, static
  `/data/skills*` catalogs/definitions, locale namespace, browser Skill service,
  definition types and executable helper library.
- Delete MessageInput and Workspace selection UI plus all store actions/caches
  that can install, edit, select or resolve pure-text Skills.
- Delete all Chat send/regenerate/edit calls to the legacy resolver and all
  legacy prompt/Tool/system-context injection. Ordinary Chat, Search, Reasoning,
  Knowledge, files, Memory and MCP behavior remains unchanged.
- Remove frontend API/DTO/config references. Backend Chat create/update/read
  sanitizes the retired Conversation key and duplicate cannot restore it.
- Never auto-install or wrap a package from an old ID, title, name or body.

### Preserve only a retirement fact in history

- A bounded migration/schema guard may recognize non-empty historical
  `skillInvocations`, but must immediately collapse all entries and fields to
  `legacySkillRetired: true`.
- Runtime Message types expose no legacy invocation ID/title/description/
  category/mode. `MessageItem` renders exactly one non-interactive localized
  label: `旧版技能已退役`.
- The label cannot open a definition, trigger execution, match a package or
  enter provider/Tool context.

### Held Neo Runtime cutover

- Retain `/v1/skills/*`, package Store/library, Agent Center Skills/Runs/
  Schedules/Learning views, immutable fingerprints and server authority.
- With legacy execution gone, Neo Runtime is the only eligible Skill execution
  domain. On this exact host it remains unavailable and returns the stable held
  reason; no dual execute, hidden compatibility path or in-process/browser
  executor exists.
- Update Phase 0/product/cutover gates and docs to assert both the deletion and
  the honest held boundary.

## Acceptance criteria

- [x] Persistence version advances; top-level and nested settings lose exactly
      the eight retired fields, and `partialize` cannot recreate them.
- [x] Session/Workspace normalization, Chat persistence, API mapping and
      explicit database cutover leave zero `activeSkills` selection authority.
- [x] Backup/fingerprint and PostgreSQL expected-count confirmation precede any
      destructive production step; unrelated state is byte-equivalent.
- [x] No legacy editor/sidebar/URL/static catalog/service/type/store/selection or
      prompt-execution path remains.
- [x] Local/server send, regenerate and edit paths never call a legacy resolver
      or inject its prompt/Tool context.
- [x] Old invocation arrays normalize to one `旧版技能已退役` fact with no
      retained detail or executable identity; message content/history survives.
- [x] Package Skills and Agent Center remain separate from Assistant/MCP and are
      the only eligible Runtime path; unavailable Runtime remains fail closed
      with `ISOLATION_UNAVAILABLE` and no fallback.
- [x] Focused migration/history/negative-reference/PostgreSQL tests, frontend
      format/lint/typecheck/Vitest/build, backend vet/tests, Phase 0, security and
      full standalone gates pass.
- [x] Reload/restart and repeated migration prove zero resurrection. Production
      enablement remains held until exact-host isolation/backup/canary/rollback
      evidence exists.

## Definition of done

- Source, tests, migration/cutover scripts, locales, architecture/contracts/
  deployment/tracking and Trellis specs agree on the one-way deletion.
- No protected runtime path or live environment file is touched.
- Work commit, task archive and journal commits are created without amend/push.

## Technical approach

Use a layered retirement guard: pure targeted browser strip functions, an
explicit raw localStorage/IndexedDB migration with last-write marker and
compensation, defensive Zustand migrations/normalizers after version bump, and
an explicit backup-gated PostgreSQL JSONB-key cutover rather than a misleading
reversible schema migration. Delete legacy runtime code/assets wholesale.
Collapse historical invocation detail at parse and browser normalization
boundaries into a boolean fact.

## Decision (ADR-lite)

**Decision:** Delete rather than convert. Preserve only history retirement
semantics. Keep package Runtime authority server-side and held on the current
host.

**Consequences:** Old custom prompts/selections are intentionally gone after
cutover and must be reinstalled explicitly as admitted packages if desired.
Chat loses the old browser Skill selector immediately. Rollback requires the
pre-cutover backup and previous images as one operation; there is no mixed or
hidden fallback mode.

## Out of scope

- Converting, wrapping, name-matching or automatically installing old Skills.
- Enabling production Runner/Shadow/Scheduler/Learning execution without exact
  deployment evidence.
- Assistant Store, MCP Tool, Knowledge, Memory, file or general Chat redesign.
- Child depth above one, rootful execution, browser/API package execution or
  G20.10 operational closure.

## Research references

- [`research/legacy-authority-inventory.md`](research/legacy-authority-inventory.md)
- [`research/persistence-hard-delete.md`](research/persistence-hard-delete.md)
- [`research/history-retirement-rendering.md`](research/history-retirement-rendering.md)
- [`research/held-runtime-cutover.md`](research/held-runtime-cutover.md)
