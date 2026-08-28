# LobeHub live Skill response shape

Observed through the configured server-side M2M transport on 2026-08-28. No
credential or response body containing package content was persisted.

## Category endpoint

- Path: `/api/v1/skills/categories?locale=zh-CN`
- Top-level JSON type: array
- Current cardinality: 296
- Row keys: `category`, `count`
- The existing parser rejects the entire response because its hard ceiling is
  256; the search path deliberately ignores this auxiliary failure, leaving the
  frontend with only its synthetic `All` entry.

## List endpoint

- Top-level keys include `categories`, `currentPage`, `items`, `pageSize`,
  `totalCount`, and `totalPages`.
- `categories` is a 296-entry string array without counts.
- First-page summaries expose `icon` as an HTTPS URL. The inspected samples
  used `github.com` URLs.

## Existing project pattern

`McpServerIcon` already accepts only HTTPS images (or at most four text code
points), lazy-loads with `referrerPolicy="no-referrer"`, and falls back to a
local Lucide icon after image failure. Skill Store must mirror that visible
behavior but cannot reuse its direct `unoptimized` request because the Skill
Store contract forbids browser-to-GitHub calls. The current live icon shape is
exactly `https://github.com/<owner>.png`, which can be narrowly allowlisted for
the same-origin Next.js image optimizer.

## Implementation constraints

- Keep a finite category ceiling and malformed-row filtering.
- Do not make category failure fatal to the item list.
- Keep server-side LobeHub authentication and direct package download
  boundaries unchanged.
- Do not run unrelated full test suites for this localized repair.
