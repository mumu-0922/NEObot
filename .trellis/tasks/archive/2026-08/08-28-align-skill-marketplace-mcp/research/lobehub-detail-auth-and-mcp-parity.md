# LobeHub Detail Auth and MCP Marketplace Parity

## Live Protocol Finding

- `GET /api/v1/skills?...` succeeds with the existing Neo Chat M2M bearer.
- `GET /api/v1/skills/{identifier}?locale=zh-CN` with the same bearer returns
  `401 invalid_token`.
- The exact detail request without Authorization returns `200` and the expected
  detail fields, including manifest, resources, versions, source repository,
  rating, and overview.
- Reproduced with `bytedance-deer-flow-find-skills` on 2026-08-28.

## Boundary Decision

- List/categories: authenticated M2M transport.
- Detail: bounded public transport to the configured LobeHub HTTPS origin.
- Download/install: authenticated exact-version download followed by the
  existing immutable archive validation and owner-private installation path.

## UX Mapping

The existing MCP Marketplace already provides the target pattern:

- full-width search header;
- category/count rail;
- responsive card grid with source summary;
- incremental pagination;
- isolated search/detail/install errors;
- detail overlay with focus restoration.

Skill Marketplace should share this structure and behavior without merging MCP
and Skill domain models or weakening either installation boundary.
