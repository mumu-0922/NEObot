# Simplify MCP Tool Call Timeline

## Goal

Make completed MCP activity understandable to ordinary users. The process trace
must answer which MCP was used, what it did, whether it ultimately succeeded,
and how long it took without exposing internal Server references or protocol
classification as the primary UI.

## What I Already Know

- The current `ProcessTracePanel` renders raw `detail.server`, `toolName`,
  `classification`, `callStatus`, and `argumentSummary` for every MCP step.
- Backend process events currently carry only `ServerRef.Key()` such as
  `private:<uuid>`; a generic frontend cannot reliably recover the installed
  MCP display name from that identifier.
- The outer process status is authoritative for the current step. Rendering a
  stale/intermediate inner `queued` badge beside outer `成功` produces a
  contradictory completed row.
- Future Marketplace installs require a generic solution; hard-coding Context7,
  DeepWiki, or individual Tool names is not acceptable.

## Requirements (Evolving)

- Backend includes the bounded current Server display name in MCP process trace
  detail while retaining the internal reference only as optional diagnostic
  metadata.
- Default Tool rows show `<MCP display name> · <readable Tool action>`, final
  status, and duration.
- Reviewed Runner installs use the current local artifact display name rather
  than a verbose or stale Marketplace title, and a Tool-only trace summary
  reports the number of Tool calls instead of claiming a Direct answer.
- Never show `private:<uuid>`, `unknown`, raw call-state tokens, or argument
  schema in the default view.
- Humanize unknown Tool identifiers generically (`resolve-library-id` ->
  `Resolve library id`) instead of maintaining product-specific translations.
- Preserve failure, cancellation, and `outcome_unknown` visibility and do not
  weaken Backend authority or redaction.
- Apply equivalent behavior and copy in Chinese, English, and Japanese.

## Acceptance Criteria (Evolving)

- [x] A DeepWiki call is identified as DeepWiki rather than `private:<uuid>`.
- [x] A Context7 call is identified as Context7 and its Tool action is readable.
- [x] A completed call shows one consistent final status and duration.
- [x] Default rows contain no UUID, `UNKNOWN`, raw `queued`, or parameter schema.
- [x] Active, failed, canceled, and outcome-unknown calls remain distinguishable.
- [x] Generic future MCP/Tool names work without a hard-coded name table.
- [x] Context7 uses the reviewed artifact name and the panel summarizes four
  MCP calls as four Tool calls rather than `直接回答`.
- [x] Focused backend process-trace and frontend process-trace tests pass.

## Definition of Done

- Focused Go and Vitest regression tests pass.
- Frontend lint, formatting, and typecheck pass for touched files.
- MCP frontend/backend specs document the user-facing versus diagnostic split.
- Runtime frontend/backend images are rebuilt and the relevant services are
  recreated for manual verification.

## Technical Approach

Carry a bounded `serverName` through `ExecutionEvent` into the durable process
trace. Render a plain-language primary row from `serverName` and a generic
humanized Tool identifier. Use only the outer normalized process status in the
primary row. Internal Server reference, classification, raw call status, and argument
summary remain non-rendered diagnostic data only.

## Decision (ADR-lite)

**Context**: Raw internal MCP references and protocol fields make the primary
timeline unreadable and create contradictory status badges.

**Decision**: Remove Server references, classification, raw call status, and
argument schemas from the product UI entirely. Keep the bounded fields in the
server-owned trace for internal compatibility and diagnostics, but do not add a
user-facing technical disclosure.

**Consequences**: The timeline becomes immediately readable. Operators use
backend diagnostics rather than exposing implementation identifiers to end
users.

## Out of Scope

- Renaming MCP Servers or Tools in storage.
- Translating every third-party Tool identifier with a hard-coded dictionary.
- Changing Tool execution, authorization, scheduling, or result persistence.
- Redesigning non-MCP Knowledge/Web/Reasoning steps.

## Technical Notes

- Frontend renderer: `mm-chat/frontend/src/components/content/ProcessTracePanel.tsx`
- Frontend normalization/tests: `mm-chat/frontend/src/lib/chat/processTrace.ts`,
  `mm-chat/frontend/src/__tests__/processTrace.test.ts`
- Backend event bridge: `mm-chat/backend/internal/chat/mcp_tool_loop.go`,
  `mm-chat/backend/internal/chat/process_trace_runtime.go`
- Applicable specs: `.trellis/spec/frontend/mcp-tools.md`,
  `.trellis/spec/backend/mcp-tools.md`
