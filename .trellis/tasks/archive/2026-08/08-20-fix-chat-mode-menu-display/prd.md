# Fix Chat/Agent Mode Menu Display

## Goal

Fix the composer Chat/Agent mode dropdown so each option's title and
description render as a readable two-line row without overlapping the next
option or escaping the menu width.

## What I already know

- The screenshot shows the `MessageInput` Chat/Agent mode dropdown.
- Both options render a title plus a description, but
  `DropdownMenuRadioItem` has a fixed `h-8` height intended for single-line
  rows, so the second line overlaps the following option.
- The affected markup is in
  `mm-chat/frontend/src/components/chat/MessageInput.tsx`.
- The shared primitive is in
  `mm-chat/frontend/src/components/ui/dropdown-menu.tsx`; other radio menus are
  single-line and should retain their current compact height.

## Confirmed Scope

- Use a scoped style override on the two Chat/Agent items rather than changing
  every radio menu in the application.
- Preserve current copy, menu width, selection behavior, disabled behavior,
  keyboard interaction, light/dark themes, and localization.

## Requirements

- Give both Chat/Agent option rows enough height and vertical padding for their
  two-line content.
- Keep the title and description contained within the available row width and
  allow localized descriptions to wrap safely.
- Align icons and the radio indicator cleanly with multiline content.
- Do not alter unrelated dropdown items or mode-selection behavior.

## Acceptance Criteria

- [x] Chat and Agent titles/descriptions do not overlap at the screenshot's
      desktop scale.
- [x] Long Chinese, English, and Japanese descriptions remain inside the menu
      and wrap without horizontal overflow.
- [x] Both rows preserve selection, disabled, focus, and keyboard behavior.
- [x] Focused frontend tests, lint, typecheck, and formatting checks pass.

## Definition of Done

- Focused Vitest coverage is added or updated for the layout contract.
- Frontend format check, lint, typecheck, and relevant tests pass.
- No documentation update is needed unless behavior beyond presentation
  changes.

## Out of Scope

- Redesigning the composer toolbar or changing Chat/Agent semantics.
- Changing translations or globally increasing all dropdown row heights.
- Backend, persistence, or deployment changes.

## Technical Approach

Apply a feature-scoped multiline item class to both Chat/Agent
`DropdownMenuRadioItem` instances. The class will replace the primitive's fixed
height with content-driven minimum height/padding, align row contents at the
top, keep icons from shrinking, and set the text container to `min-w-0` with
safe wrapping. Add a focused source-composition assertion matching the existing
test style.

## Decision (ADR-lite)

**Context**: The shared radio primitive is optimized for single-line rows, but
this menu is the only current radio menu with title/description content.

**Decision**: Override layout only for the two multiline mode rows.

**Consequences**: The regression is fixed without widening the visual impact to
other menus. Future multiline radio menus should deliberately opt into the same
pattern or promote it to a named primitive variant.

## Technical Notes

- Root cause: fixed `h-8` on `DropdownMenuRadioItem` with two rendered text
  lines.
- Relevant specs: `.trellis/spec/frontend/component-guidelines.md`,
  `.trellis/spec/frontend/chat-agent-mode.md`, and
  `.trellis/spec/frontend/quality-guidelines.md`.
- Existing test: `mm-chat/frontend/src/__tests__/messageInputComposition.test.ts`.
