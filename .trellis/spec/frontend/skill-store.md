# Skill Store

## 1. Scope / Trigger

Apply when changing the top-level Skill Store, `/v1/skills/*` frontend client,
curated catalog rendering, installed-library management, or Skill Store URL
state. Runs, Schedules, Learning, Shadow, Canary, delegation, Runner, and OCI
controls are retired product concepts and must not reappear here.

## 2. Signatures

```ts
skillStore.listCatalog() -> {
  items: SkillCatalogSummaryDTO[];
  totalCount: number;
  source: string;
}
skillStore.getCatalogSkill(name) -> SkillCatalogItemDTO
skillStore.installCatalogSkill({
  id, resolvedCommit, packageFingerprint,
}) -> AgentPackageInstallationDTO
skillStore.searchMarketplace({ query, category, page, pageSize })
  -> SkillMarketplaceSearchResultDTO
skillStore.getMarketplaceSkill(identifier, { version? })
  -> SkillMarketplaceDetailDTO
skillStore.installMarketplaceSkill({ identifier, version })
  -> AgentPackageInstallationDTO
skillStore.installSkillLink({ url }) -> AgentPackageInstallationDTO
skillStore.listPackageLibrary() -> AgentPackageInstallationDTO[]
skillStore.uninstallPackageSkill({ installationId, revision }) -> void

panel=skill-store&skillId=<validated-curated-name|lobehub:identifier>
```

## 3. Contracts

- The page exposes mutually exclusive `Installed | Skill Store` tabs. Plain
  navigation opens Installed; an initial search or valid `skillId` opens the
  Store. Installing refreshes both authorities, clears `skillId`, and returns
  to Installed.
- The Store tab exposes mutually exclusive `LobeHub | OpenAI Curated` sources.
  LobeHub is backend-owned paginated search/category/detail; OpenAI stays the
  fixed curated catalog. Source-specific state and failures must not overwrite
  the other source or Installed inventory.
- LobeHub requests forward the active supported UI locale and a typed sort
  selection; neither field becomes an arbitrary upstream query parameter.
- The browser never calls GitHub/LobeHub directly, scrapes HTML, receives M2M
  credentials/package bytes, or treats list presence as install authority.
- Strict Zod schemas bind every catalog entry to repository `openai/skills`,
  ref `main`, path `skills/.curated/<name>`, a coherent canonical source URL,
  and matching `id`/`name`/path identity.
- Strict LobeHub Zod schemas bind identifier, exact SemVer, counts, HTTPS
  links, manifest name, permissions/resources/version arrays, source literal,
  and installed state. Unknown/malformed projections fail closed.
- Detail provides the exact resolved commit and package fingerprint needed by
  install, but normal UI presents human-facing name, description, source,
  repository path, compatibility, declared tools, and install state. It does
  not display admission IDs, raw fingerprints, reviewer/SBOM internals, package
  HTML, files, Runner data, credentials, or control-plane state.
- Installed and Store loading/error state are isolated. Catalog failure cannot
  hide or disable Installed management.
- Desktop keeps list/detail visible; mobile drills into detail and restores
  focus to the originating item on Back. Status uses accessible names and live
  announcements, with existing dark/responsive behavior preserved.
- URL `skillId` accepts a validated curated Skill name or source-qualified
  `lobehub:<identifier>`. Prefixing prevents cross-source identity collisions.
  Legacy candidate IDs
  remain parseable only for navigation compatibility. Invalid, inaccessible,
  or deleted values are removed with `replaceState` and return to the list.
- Old `panel=agent-center&agentTab=skills` URLs migrate to Skill Store; other
  legacy control-plane URL state is discarded.
- Installing changes Library inventory only. Durable per-Conversation selection
  remains owned by `ConversationResourcePickers` and is never changed
  implicitly by this page.
- Local browser mode reports this server-owned feature as unsupported.
- The link form accepts exact LobeHub Skill pages and GitHub tree/blob Skill
  paths only. Success refreshes Library and returns to Installed without
  enabling the Skill in any Conversation.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| malformed catalog/list/detail DTO | `INVALID_SERVER_RESPONSE`; render bounded retry state |
| catalog detail unavailable | detail-only error/retry; Installed remains usable |
| install returns `SKILL_PACKAGE_CHANGED` | reload exact detail before another install |
| stale uninstall revision | reload Library authority; no optimistic deletion |
| invalid/out-of-panel `skillId` | remove with `replaceState`; render owning list |
| selected detail absent from initial list | fetch exact validated `skillId`; do not infer non-existence |
| catalog unavailable | show Store failure only; Installed list remains operational |
| malformed LobeHub list/detail DTO | `INVALID_SERVER_RESPONSE`; source-bounded retry |
| Marketplace detail version changes | surface `SKILL_PACKAGE_CHANGED`; reload exact detail |
| invalid `lobehub:` URL identity | remove `skillId` with `replaceState`; do not request detail |
| unsupported/ambiguous external URL | keep Library unchanged; announce bounded install failure |

## 5. Good / Base / Bad Cases

- **Good:** browse `openai/skills`, inspect one validated Skill, install it,
  return to Installed, then independently select it for one Conversation.
- **Good:** search LobeHub by category, open a `lobehub:<identifier>` deep link,
  install the displayed exact version, and return to Installed.
- **Base:** catalog is unavailable, but the user can still inspect and remove
  already-installed Skills.
- **Bad:** expose fingerprint/SBOM/reviewer details as product copy, scrape
  GitHub from the browser, couple Store and Installed loading, or silently
  enable a newly installed Skill in every Conversation.
- **Bad:** merge OpenAI and LobeHub rows by unqualified name, download packages
  in the browser, or submit arbitrary webpages/commands to direct install.

## 6. Tests Required

- Skill Store tab composition, Installed/Store separation, curated catalog API
  calls, post-install return, and absence of retired/internal control terms.
- Strict catalog DTO identity, detail/install fingerprint binding, revision-
  bound uninstall, malformed response, and stale-package behavior.
- Strict LobeHub search/detail/install/direct-link routes, malformed source
  identity, exact-version body, categories/pagination, and HTTPS-only URLs.
- URL round-trip, direct curated detail, invalid path rejection, and legacy
  Agent Center Skills migration.
- Source-qualified LobeHub URL round-trip and encoded path rejection.
- Focus restoration, accessible live feedback, responsive detail, catalog-
  failure isolation, format, lint, typecheck, Vitest, and production build.

## 7. Wrong vs Correct

```text
Wrong: UI -> GitHub/marketplace scrape -> trust response -> install
Correct: UI -> strict /v1/skills source DTO -> validated exact detail -> fenced install

Wrong: merge sources by Skill name -> select/install the wrong package
Correct: keep source state separate -> use lobehub:<identifier> URL identity

Wrong: install success -> persist Conversation selection
Correct: install success -> refresh Library; composer owns selection

Wrong: catalog request fails -> replace the whole page with failure
Correct: Store error is isolated; Installed remains independently operational
```
