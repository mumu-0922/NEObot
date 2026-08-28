# Rename Skill and MCP product labels

## Goal

Rename the two primary navigation/product areas so the Chinese UI presents
`技能商店` as `Skill` and `工具` as `MCP`.

## What I already know

- The screenshot shows the affected labels in the left navigation and the MCP
  page title.
- The strings are owned by the Chinese `Sidebar`, `SkillStore`, `Mcp`, and
  composer resource-management locale files.
- Generic references to individual MCP Tools, Tool calls, Tool counts, and
  unrelated programming utilities are not product-area labels.

## Requirements

- Change the Skill product-area title, tab, navigation entry, and associated
  open/close/loading labels from `技能商店` to `Skill`.
- Change the MCP product-area title, navigation entry, and associated
  open/close/loading labels from `工具` to `MCP`.
- Keep generic Tool terminology such as `工具调用`, `{count} 个工具`, and Tool
  previews unchanged.
- Update focused locale/composition tests.

## Acceptance Criteria

- [x] The left navigation displays `Skill` and `MCP`.
- [x] The Skill page title/store tab displays `Skill`.
- [x] The MCP page title displays `MCP`.
- [x] Generic MCP Tool counts and Tool-call copy remain unchanged.
- [x] Focused frontend tests, formatting, lint, and typecheck pass.

## Definition of Done

- Locale source and focused tests agree.
- The active Frontend is rebuilt and the live page reflects the renamed labels.
- A focused conventional commit is created and the task is archived.

## Out of Scope

- Renaming backend API types, MCP protocol entities, route names, or database
  state.
- Replacing every ordinary Chinese word `工具` in the application.
- Redesigning icons, layout, spacing, or interaction behavior.

## Technical Notes

- Primary locale files: `src/i18n/locales/zh/Sidebar.json`,
  `src/i18n/locales/zh/SkillStore.json`, `src/i18n/locales/zh/Mcp.json`, and
  `src/i18n/locales/zh/MessageInput.json`.
- This is a trivial copy-only frontend change; no external research is needed.
