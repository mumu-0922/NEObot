# Layout Components

Layout components define app navigation, workspace structure, and global shell behavior.

## Files

- `Sidebar.tsx` renders session navigation, workspace navigation, pinned sessions, and primary app actions.
- `WorkspaceSettingsModal.tsx` manages workspace metadata, preset files, workspace-level settings, active plugin presets, and active skill presets.

## Guidelines

- Keep layout state separate from chat-domain mutations when possible.
- Preserve keyboard and focus behavior in navigation and modal flows.
- Keep workspace file logic aligned with `src/lib/utils/workspaceFiles.ts`.
- Normalize workspace skill presets against the installed skill list before saving.
- Expanded primary navigation rows already expose text and must render without
  a duplicate Tooltip. Collapsed rows may use an instant solid hint, and their
  direct hover highlight must not interpolate color or background state.
