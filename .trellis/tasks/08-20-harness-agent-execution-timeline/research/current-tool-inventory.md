# Current neo-chat Agent Tool inventory

## Tool families

- Retrieval: `search_web`, `search_knowledge`, `search_memory`.
- Local direct: `skill`, `file_read`, `file_write`, `file_edit`, `file_search`,
  `publish_file`, `job_list`, `job_output`, `job_kill`, `terminal`.
- Compatibility-only local names: `skills_list`, `skill_view` remain executable
  during retirement but are not exposed to new models.
- Goal: `get_goal`, `create_goal`, `update_goal`, `verify_completion`.
- MCP: dynamically authorized Tools plus `neo_mcp_tool_search` when the selected
  Tool set is too large.
- Browser: the selected Playwright manifest is an MCP server with a precise
  17-Tool allowlist; it is not a separate backend Tool kind.
- Provider built-in Web Search produces Web process steps/search results rather
  than a normal local Tool result.

## Existing event and UI boundaries

- `ChatAgentEvent` already carries event, turn, conversation, message, run,
  sequence, type, optional step sequence, payload, and occurrence time.
- Tool event payloads deliberately omit raw arguments/results and project into
  `ProcessStep`.
- `ProcessStep` supports reasoning, knowledge, web, tool, and generation kinds,
  plus a bounded detail allowlist. Only Terminal currently has a specialized
  `presentation` card.
- `ProcessTracePanel.tsx` renders the server Agent timeline. Most Tools remain a
  generic single row.
- `ToolCallBlock.tsx` renders legacy/client-side Tool calls. Although server SSE
  parses `tool.call.updated`, the server chat store does not connect that
  callback, so it must not be treated as the new authority.

## Result and error notes

- Local File/Terminal/Job results contain the facts required for safe typed
  presenters, but they currently return to the provider rather than the
  durable UI projection.
- MCP results are typed text/JSON/media/resource projections and already move
  oversized binary content toward artifacts.
- MCP and local Tool layers expose normalized failure categories, but the
  frontend currently collapses most categories into a generic error label.
- The live database contains an unused old private Playwright snapshot with
  unsafe Tool names. It is not currently selected; rollout must quarantine it
  before any audited removal rather than treating it as the callable surface.

