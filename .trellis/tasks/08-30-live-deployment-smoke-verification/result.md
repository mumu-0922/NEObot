# Live Deployment Smoke Result

## Deployment proof

- Same-origin entrypoint: healthy (`frontend=200`, `backend health=200`).
- Runtime services: Backend, Frontend, MCP Runner, Memory Worker, PostgreSQL,
  RAG Worker, and Redis healthy; MinIO running.
- Authentication: one short-lived canonical Session, revoked and removed after
  the run without changing credentials or Provider configuration.
- Provider: existing server-default Sub route with `gpt-5.6-luna`; no Provider
  descriptor, secret, request body, or private result was recorded.

## Bounded live paths

| Path | Terminal evidence | Persisted reload | Duration |
| --- | --- | --- | ---: |
| Chat | completed Assistant message | passed | 2.7 s |
| Agent | completed harmless Tool step and answer | passed | 8.4 s |
| RAG | `search_knowledge` evidence | passed | 9.6 s |
| Memory | `search_memory` evidence | passed | 5.6 s |
| Skill | local Skill Tool evidence | passed | 8.3 s |
| MCP | Context7 `resolve-library-id` and `query-docs` succeeded | passed | bounded |

The original MCP harness display matcher produced a false negative because the
server-list projection omitted Tool details. The durable MCP execution log
proved both calls reached `succeeded`; the product path passed.

## Defect and repair

Conversation deletion is a soft delete. MCP selection cleanup was explicit,
but Skill selection cleanup incorrectly relied on hard-delete foreign-key
cascade, leaving one hidden `skill_conversation_selections` row. The repair
adds owner-bound transactional Skill cleanup before Conversation soft deletion
and fails closed if cleanup cannot commit. Installed Skill inventory remains.

## Cleanup proof

- Active smoke Conversations: `0`
- Inaccessible soft-deleted audit/history rows: `6`
- Smoke Sessions: `0`
- Skill selections: `0`
- MCP selections: `0`
- Memory jobs: `0`

## Verification

- Focused Skill/Chat regression tests: passed.
- Complete `internal/skillsupply` and `internal/chat` tests: passed.
- Backend `go vet ./...`: passed.
- Backend `go test ./...`: passed.
- Skill supply race/runtime verification: passed.
- Disposable PostgreSQL 17 Skill supply migration/ownership/CAS/lifecycle
  drill through schema head 109: passed.
- `git diff --check`: passed.

No deployment rebuild, restart, migration, Provider change, or existing user
resource mutation was performed.
