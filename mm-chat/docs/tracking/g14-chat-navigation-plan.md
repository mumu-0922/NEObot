# G14 Chat Navigation Plan

Status: complete. Work was split into independently reversible slices; each
slice must pass focused tests, the full frontend gate, a source-image rebuild,
and a deployed Windows Chrome smoke before commit.

## Reference boundary

`zichen0116/ChatGPT-nav` commit
`0d1c4dfc56221ca39a4790d22e7a36bb2ce9c3ca` is used as an interaction
reference only. Its useful behaviors are a narrow right-side rail, hover/focus
expansion, user-message extraction, click navigation, and current-reading
highlight. `mm-chat` must implement these behaviors as typed React components
against its own message tree and scroll container; no userscript DOM observer
or copied runtime is introduced.

## Slice sequence

### [x] G14.1 — Draggable chat scrollbar

- Give the main conversation scroll root a persistent, visible native vertical
  scrollbar on the far right.
- Keep the scrollbar track clickable and its thumb mouse-draggable.
- Preserve manual-scroll pause, bottom-follow resume, composer clearance, and
  the existing long-message geometry fixes.

### [x] G14.2 — User-message navigation and edge jumps

- Add a compact right-side user-message rail that expands on hover or keyboard
  focus without covering the native scrollbar.
- Build entries from the active branch's user messages, truncate labels, expose
  full accessible names, and highlight the message nearest the reading line.
- Navigate to a selected user message, including a message not yet mounted by
  progressive rendering.
- Add explicit jump-to-top and jump-to-bottom controls and keep scroll-follow
  state consistent with each action.
- Keep the rail out of constrained mobile layouts while retaining the native
  scrollbar and existing touch scrolling.

### [x] G14.3 — Per-item navigation title preview

- Keep the desktop rail fixed at `44px`; hovering or focusing the rail must not
  expand the whole navigation list.
- Show exactly one floating title for the currently hovered or keyboard-focused
  message or edge control.
- Remove the preview immediately on item mouse leave, blur, rail mouse leave, or
  navigation-list scroll, without a delayed native `title` tooltip.

## Rollback

Each slice has its own commit. G14.1 is CSS plus one scroll-root class. G14.2
adds an isolated frontend component/helper and bounded ChatApp wiring. G14.3
changes only the rail's title-preview interaction and its regression contract.
None of the slices changes APIs, Postgres, Redis, MinIO, provider calls, or
persisted chat data.
