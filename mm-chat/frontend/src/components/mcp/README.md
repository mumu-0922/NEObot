# MCP Components

`McpToolsControl.tsx` renders the conversation-level **Tools** control. It lists
authorized MCP servers, edits inherited/custom selection, disables individual
Tools, manages private remote definitions and credentials, starts OAuth, and
offers the explicit disable-all-and-continue recovery path.

The component calls the typed `/v1/mcp/*` client. It never stores credential
values, authorizes Tool calls, executes MCP, or treats browser Workspace state
as server authority.

## Usage

```tsx
<McpToolsControl
  conversationId={conversationId}
  enabled={serverConfig?.mcp.enabled === true}
  onDisableAllAndContinue={retryPendingSend}
/>
```

The parent may pass an `attention` message when pre-send validation fails. The
control owns only transient dialog/form state.
