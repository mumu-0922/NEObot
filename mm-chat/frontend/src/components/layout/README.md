# Layout Components

Layout components define app navigation, workspace structure, and global shell behavior.

## Files

- `Sidebar.tsx` renders session navigation, workspace navigation, pinned sessions, and primary app actions.
- `WorkspaceSettingsModal.tsx` manages workspace metadata, preset files, workspace-level settings, and MCP Tool defaults.

## Guidelines

- Keep layout state separate from chat-domain mutations when possible.
- Preserve keyboard and focus behavior in navigation and modal flows.
- Keep workspace file logic aligned with `src/lib/utils/workspaceFiles.ts`.
- Never persist retired Workspace `activeSkills`; normalization strips stale
  imports instead of matching them to Package Skills.
- Expanded primary navigation rows already expose text and must render without
  a duplicate Tooltip. Collapsed rows may use an instant solid hint, and their
  direct hover highlight must not interpolate color or background state.
- Root and workspace conversation rows follow the same direct-pointer rule;
  their background and more-actions reveal must not use hover transitions.
