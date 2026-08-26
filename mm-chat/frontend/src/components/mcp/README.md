# MCP Components

`McpToolsControl.tsx` renders the Sidebar **Tools/Connectors** management
surface. It lists authorized MCP servers and manages private remote definitions,
credentials, health, Marketplace installation and OAuth. It never changes a
Conversation selection. The chat composer does not embed this management
component or preflight MCP before every send.

`ConversationResourcePickers.tsx` provides the compact MCP icon beside the
composer controls. It selects already-installed, ready servers for exactly one
conversation through the existing revision-bound MCP authority. Configuration
and uninstall remain in Tools.

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
