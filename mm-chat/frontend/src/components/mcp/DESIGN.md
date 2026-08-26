# MCP Components Design

The MCP frontend has two deliberately separate surfaces:

- `McpToolsControl` and Marketplace own installed inventory, configuration,
  health, authorization, and removal.
- `ConversationResourcePickers` owns the exact current Conversation selection
  through backend revision/CAS authority.

The management surface never writes Conversation selection. Marketplace
install and OAuth use inventory-only requests, so configuration cannot silently
enable a Server in the active Conversation.

Authorization and unavailability remain visible in management and the composer
picker admits only ready servers. No per-call approval dialog is introduced.

## Design decisions and tradeoffs

- Two compact composer icons keep Skill/MCP authority visible at send time,
  while full management remains on dedicated pages.
- Conversation selection updates are immediate and revision-bound rather than
  staged in a browser draft; conflicts reload the exact Conversation authority.
- Credentials stay in component memory only, so closing/reloading loses an
  unsaved value by design.
