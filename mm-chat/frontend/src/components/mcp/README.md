# MCP Components

`McpToolsControl.tsx` renders the Sidebar **Tools/Connectors** management
surface. It lists authorized MCP servers, edits inherited/custom selection,
disables individual Tools, manages private remote definitions and credentials,
and starts OAuth. The chat composer exposes Chat/Agent mode and must not embed
this MCP-specific component or preflight MCP before every send.

The component calls the typed `/v1/mcp/*` client. It never stores credential
values, authorizes Tool calls, executes MCP, or treats browser Workspace state
as server authority.

## Usage

```tsx
<McpToolsControl
  conversationId={conversationId}
  enabled={serverConfig?.mcp.enabled === true}
  variant="embedded"
/>
```

The control owns only transient dialog/form state. Agent-mode admission errors
may direct users to this page, while Chat mode never prepares MCP.
