# Fix local Skill browser smoke

## Goal

Restore the first real browser smoke for installed `local_direct` Package
Skills: Agent Center must load for the configured single-server owner, and an
OpenAI-compatible model must accept the local Tool definitions and execute the
`skills_list -> skill_view -> terminal` loop.

## What I already know

- The user installed `workspace-smoke v1.0.0` successfully from the admitted
  Package Skill Store.
- The browser Chat request persisted a failed assistant message with
  `errorCode=LOCAL_SKILL_PROVIDER_FAILED` before any Tool execution event.
- The three local Tool definitions declare `strict=true` while omitting optional
  properties from `required`; OpenAI strict-function schemas require every
  property to be required (nullable where semantically optional).
- The recommended Provider-compatible repair keeps `strict=true`, includes
  every property in `required`, and makes semantically optional values nullable.
  Local Skills still decode and validate every Tool call locally.
- Agent Center status, Runs, and Schedules return
  `INVALID_AGENT_REQUEST/QUERY` for the configured development/bootstrap owner
  `00000000-0000-0000-0000-000000000001`.
- `agentcontrol.validUUID` rejects that repository-standard canonical
  PostgreSQL UUID because it incorrectly requires an RFC version and variant
  nibble.
- After the Backend repairs, the real retry completes and persists the final
  assistant answer, but the browser rejects the first `tool.call.updated`
  event. The shared frontend normalizer accepts only `mode=mcp` and MCP's
  `read|write|unknown` classifications, while current local Skill events use
  `mode=local_direct` and may use the `execute` classification.
- The user previously authorized recommended fixes and direct commits without
  another confirmation round.

## Assumptions

- Existing API credentials, Provider selection, Skill installation, and local
  runtime mounts remain unchanged.
- The default bootstrap/development UUID is a supported persisted identity and
  must not be migrated merely to satisfy an over-strict application regex.

## Requirements

- Accept canonical PostgreSQL UUID strings used by configured local identities,
  including the repository default development/bootstrap UUID.
- Keep non-canonical or malformed UUID input rejected.
- Do not claim OpenAI strict-schema compliance for local Skill Tool definitions
  that intentionally expose optional fields.
- Preserve local strict argument decoding, path validation, command policy,
  budgets, timeouts, and same-model continuation.
- Accept the bounded `local_direct` Tool-update mode and `execute`
  classification at the frontend stream boundary without weakening MCP Server
  definition/classification validation.
- Add regression tests for both failures.
- Rebuild/restart the affected application services and replay the browser-like
  endpoints after the fix.

## Acceptance Criteria

- [x] Agent Center status returns HTTP 200 for the configured owner and reports
      `runtime.state=local_ready`.
- [x] Runs and Schedules list endpoints return HTTP 200 instead of invalid
      request/query errors.
- [x] Local Tool definitions remain `strict=true`; every property is required,
      semantically optional values are nullable, and schemas retain
      `additionalProperties=false` plus runtime validation.
- [x] A real browser Chat retry reaches Tool execution and creates
      `/workspace/hello-skill.txt` containing `skill-ok`.
- [x] The browser consumes the `local_direct` Tool timeline and renders the
      completed answer without `invalid MCP Tool call update`.
- [x] Focused and full Backend tests pass; Frontend contract checks remain green
      if touched.
- [x] Services are healthy and the source work is committed.

## Out of Scope

- Redesigning Agent Center Runs/Schedules product semantics.
- Changing Provider credentials or model selection.
- Reintroducing an isolated per-Skill Runner.
- Weakening terminal command policy or filesystem boundaries.

## Technical Notes

- Browser evidence: screenshots at 2026-08-16 09:09 Asia/Shanghai.
- Runtime evidence: failed message metadata contains
  `LOCAL_SKILL_PROVIDER_FAILED`; no `hello-skill.txt` exists.
- Primary code paths:
  - `mm-chat/backend/internal/chat/local_skill_tool_loop.go`
  - `mm-chat/backend/internal/chat/local_skill_tool_loop_test.go`
  - `mm-chat/backend/internal/agentcontrol/service.go`
  - `mm-chat/backend/internal/agentcontrol/service_test.go`
  - `mm-chat/frontend/src/lib/mcp/types.ts`
  - `mm-chat/frontend/src/services/api/client/server/chatApi.ts`
  - `mm-chat/frontend/src/__tests__/mcpTypes.test.ts`
- Rollback is code-only: revert the fix commit and rebuild Backend. No schema or
  persisted Skill data change is required.
