# Add Skill and MCP picker icons

## Goal

Make the compact per-conversation Skill and MCP pickers visually consistent
with their installed-management surfaces by rendering the same shared icon
components beside every selectable resource.

## What I already know

- `ConversationResourcePickers.tsx` currently renders text-only Skill and MCP
  rows.
- Installed Skill cards already use the shared `SkillIcon` renderer.
- Installed MCP cards already use the shared `McpServerIcon` renderer and MCP
  inventory exposes an optional normalized `icon` field.
- Installed Skill DTOs do not carry an icon, so the shared deterministic label
  fallback is the correct source-independent presentation.

## Requirements

- Render a compact `SkillIcon` before every installed Skill picker label.
- Render a compact `McpServerIcon` before every MCP picker label, using the
  server DTO icon when present and the existing local fallback otherwise.
- Extend the shared renderers with a compact visual variant instead of
  duplicating icon parsing or fallback behavior in the picker.
- Preserve search, checked state, disabled state, revision-bound writes, and
  the single-open-popover contract.
- Add no network lookup, browser registry, schema, API, or persistence change.

## Acceptance Criteria

- [x] Skill picker rows render the same deterministic shared icon identity as
      installed Skill cards in a compact size.
- [x] MCP picker rows render the same normalized/fallback identity as installed
      MCP cards in a compact size.
- [x] Icon and two-line label remain aligned without clipping or overlapping
      checkbox affordances.
- [x] Focused composition tests prove both shared icon renderers are reused.
- [x] Changed-file formatting/lint, focused Vitest, TypeScript typecheck, and
      the frontend production build pass.
- [x] The updated frontend is deployed and the health endpoint is ready.

## Definition of Done

- Focused tests and static checks pass.
- Frontend behavior specs document picker icon reuse.
- A focused conventional commit is created.

## Out of Scope

- Adding artwork fields to installed Skill DTOs.
- Changing picker triggers, selection semantics, marketplace behavior, or
  management-page layouts.
- Full repository/component test suites for this localized presentation change.

## Technical Notes

- Owning component: `mm-chat/frontend/src/components/chat/ConversationResourcePickers.tsx`.
- Shared renderers: `components/skills/SkillIcon.tsx` and
  `components/mcp/McpServerIcon.tsx`.
- Tests live under `mm-chat/frontend/src/__tests__/`.
