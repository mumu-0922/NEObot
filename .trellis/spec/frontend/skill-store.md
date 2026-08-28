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

- The page exposes mutually exclusive `Installed | Skill` tabs. Plain
  navigation opens Installed; an initial search or valid `skillId` opens the
  Store. Installing refreshes both authorities, clears `skillId`, and returns
  to Installed.
- `Skill` is the product-area label in every supported locale; generic Skill
  descriptions and source-specific Marketplace names remain unchanged.
- The Store tab exposes mutually exclusive `LobeHub | OpenAI Curated` sources.
  LobeHub is backend-owned paginated search/category/detail; OpenAI stays the
  fixed curated catalog. Source-specific state and failures must not overwrite
  the other source or Installed inventory.
- LobeHub requests forward the active supported UI locale and a typed sort
  selection; neither field becomes an arbitrary upstream query parameter.
- The browser never calls GitHub/LobeHub directly, scrapes HTML, receives M2M
  credentials/package bytes, or treats list presence as install authority.
- LobeHub Skill avatars render only from the validated
  `https://github.com/<owner>.png` shape through the same-origin Next.js image
  optimizer. Other URLs remain local fallbacks; the browser never requests an
  upstream icon directly.
- Store and Installed cards use the same `SkillIcon` renderer. It prefers the
  validated GitHub avatar shape, then a bounded short emoji, then the uppercase
  first character of the display label, and finally the package glyph. An
  installation DTO without an icon must still render that deterministic local
  fallback; Library rendering must not call Marketplace or require a schema
  migration merely to recover presentation artwork.
- The per-Conversation composer picker reuses the same `SkillIcon` renderer in
  its compact variant. Picker rows must not duplicate icon parsing, fetch
  Marketplace artwork for installed DTOs, or change selection authority.
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
- The LobeHub Store follows the MCP Marketplace structure: full-width search,
  responsive category rail, two-column card grid, incremental pagination, and
  a modal detail layer. Detail close/Escape restores focus to the originating
  card and never discards the current result grid. Status uses accessible names
  and live announcements, with existing dark/responsive behavior preserved.
- The category rail renders only the backend-owned 21 canonical LobeHub IDs in
  their product-defined order. Display labels come from the `SkillStore`
  Chinese, English, and Japanese locale maps and each category uses its own
  semantic Lucide icon; raw source slugs and the generic folder icon are not
  user-facing copy. Selection still sends the exact canonical ID to the backend
  and never filters only the currently loaded cards.
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
| category projection contains more than 21 rows | `INVALID_SERVER_RESPONSE`; source-bounded retry |
| Marketplace detail version changes | surface `SKILL_PACKAGE_CHANGED`; reload exact detail |
| invalid `lobehub:` URL identity | remove `skillId` with `replaceState`; do not request detail |
| unsupported/ambiguous external URL | keep Library unchanged; announce bounded install failure |

## 5. Good / Base / Bad Cases

- **Good:** browse `openai/skills`, inspect one validated Skill, install it,
  return to Installed, then independently select it for one Conversation.
- **Good:** search LobeHub by category, open a `lobehub:<identifier>` deep link,
  install the displayed exact version, and return to Installed.
- **Good:** render `coding-agents-ides` as the active locale label with the
  Code icon while submitting the unchanged ID to the backend.
- **Good:** render an existing installed `pdf` package with the same local `P`
  fallback used by its curated Store card, without reinstalling it.
- **Base:** catalog is unavailable, but the user can still inspect and remove
  already-installed Skills.
- **Base:** a long-tail category is absent from the rail but its Skills remain
  discoverable through All and keyword search.
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
- Category-rail/card-grid composition, modal focus restoration, accessible live
  feedback, responsive detail, catalog-failure isolation, format, lint,
  typecheck, Vitest, and production build.
- Category composition tests must assert exactly 21 distinct icon definitions,
  locale coverage for every ID, absence of raw-label rendering, and the strict
  21-row DTO ceiling.
- Icon composition tests must assert that Store and Installed cards import the
  one shared renderer, that label fallback remains deterministic, and that the
  GitHub-only remote-image allowlist remains unchanged.

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

Wrong: render raw category slug with one Folder icon for every row
Correct: exact category ID -> localized label + category-specific Lucide icon

Wrong: Installed card omits identity because its DTO has no source artwork
Correct: reuse SkillIcon -> render deterministic local label fallback offline
```
