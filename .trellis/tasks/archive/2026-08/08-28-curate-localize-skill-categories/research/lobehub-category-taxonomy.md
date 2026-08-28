# LobeHub Skill category taxonomy

Sanitized observation through the configured server-side M2M transport on
2026-08-28. No credential or package content was persisted.

## Findings

- The category endpoint currently returns 296 rows.
- The high-volume head is coherent and stable-looking: `coding-agents-ides`
  (51,212), `devops-cloud` (41,870), `web-frontend-development` (34,056),
  `cli-utilities` (27,140), and `productivity-tasks` (22,563).
- The long tail contains metadata and community-defined labels such as `_meta`,
  `_persona`, `Backend`, `Backend & Infrastructure`, mixed-case duplicates,
  framework names, testing bundles, and categories with single-digit counts.
- Category filtering accepts one exact category ID. A synthetic group would
  require multiple upstream queries and merge/paging semantics, so it is not a
  safe localized UI-only change.

## Selected product taxonomy

The rail keeps 21 exact, high-signal IDs in product-defined order:

1. `coding-agents-ides`
2. `devops-cloud`
3. `web-frontend-development`
4. `cli-utilities`
5. `productivity-tasks`
6. `ai-llms`
7. `git-github`
8. `data-analytics`
9. `marketing-sales`
10. `search-research`
11. `self-hosted-automation`
12. `agent-to-agent-protocols`
13. `finance`
14. `communication`
15. `notes-pkm`
16. `image-video-generation`
17. `browser-automation`
18. `ios-macos-development`
19. `security-passwords`
20. `gaming`
21. `pdf-documents`

Together these cover the meaningful head of the live catalog while keeping the
rail comparable in density to the MCP Marketplace. `All` and keyword search
retain access to the omitted long tail.

## Boundary decision

- Backend filters/validates exact IDs and preserves live counts.
- Frontend translates labels and maps IDs to semantic Lucide icons.
- No current-page-only filtering and no synthetic count aggregation.
