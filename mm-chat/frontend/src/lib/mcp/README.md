# MCP Frontend Library

This directory contains frontend-only MCP DTOs. `types.ts` mirrors the public
camelCase `/v1/mcp/*` shapes and the bounded chat-stream call update.

No file in this directory owns server definitions, grants, credentials,
execution, retries, results, or authorization.

## Usage

```ts
import type { McpConversationSelection } from "@/lib/mcp/types";

const explicitlyDisabled =
  selection.mode === "custom" && selection.servers.length === 0;
```

Existing chat process-trace code renders backend call updates; do not
reconstruct a call state from message text.
