# Dynamic Knowledge routing result

## Outcome

- `630fb80` implemented the bounded, query-aware Knowledge catalog, governed
  metadata disclosure, native Auto Tool routing, unified compatibility
  planning, and shared provider/model Tool-capability state.
- Migration `042_model_tool_capability_cache` stores only derived
  `supported|unsupported|unknown` probe state with bounded TTLs and
  provider-config-hash isolation.
- Frontend provider settings expose default/model overrides, and the process
  panel derives query-free `Direct|Knowledge|Web|Both` summaries.

## Automated evidence

- Catalog tests cover CJK bigrams, English terms, five relevant plus three
  representative filenames, UTF-8/4 KiB bounds, eight-collection enforcement,
  ACL filtering, active-file filtering, and representative-title non-authority.
- Planner/Handler tests cover Direct, Knowledge, Web, Both, miss behavior,
  deterministic failure fallback, cancellation, runtime incompatibility
  downgrade, stable citations, and exact-query/catalog redaction.
- Capability tests cover override precedence, probe payload isolation,
  structured-call classification, non-blocking singleflight, TTL expiry,
  config-hash invalidation, PostgreSQL sharing, and DTO/UI round trips.
- Current `go vet ./...`, focused Go packages, and `18 files / 196` focused
  frontend tests passed. The later full standalone gate also passed the entire
  current product tree.

## Live routing evidence — 2026-07-27

A temporary conversation used the selected task collection and
`SERVER_DEFAULT:gpt-5.6-sol`:

- `有小作文模板嘛` completed Generation, Tool, and Knowledge steps with
  `knowledgeOutcome=answered` and one durable Knowledge citation.
- `帮我写一句生日祝福` completed Generation only: Direct, with no Knowledge or
  Web execution.
- `今天 OpenAI 有什么新消息` completed Generation, Tool, and Web steps without
  Knowledge taking authority.
- Both temporary smoke conversations were deleted successfully with HTTP 204.

## Rollback

Revert `630fb80`. For a narrow routing-quality rollback, omit
`WithKnowledgeRoutingCatalog`; migration `042` can be rolled down because it
contains derived cache state only.
