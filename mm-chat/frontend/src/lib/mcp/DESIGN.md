# MCP Frontend Library Design

The library intentionally stays declarative. API DTOs preserve stable server
references, selection modes, Tool compatibility, and call states. Timeline
mapping emits only redacted summaries supplied by the backend and keeps result
content collapsed outside the process trace.

Adding a new MCP state requires synchronized Go DTO, frontend type, API-client,
locale, and focused test changes. Existing process-trace mapping consumes the
bounded stream DTO; never add a browser MCP transport or local execution
fallback here.

## Design decisions and tradeoffs

- DTOs mirror backend camelCase shapes instead of introducing a second local
  model, reducing drift at the cost of explicit API-version coordination.
- Timeline state accepts only backend-redacted updates; richer result display
  must use a separately authorized API rather than retained raw client data.
