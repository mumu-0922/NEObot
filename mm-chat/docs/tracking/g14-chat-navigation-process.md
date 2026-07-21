# G14 Chat Navigation Process

## 2026-07-21 — Reference and current-runtime inspection

The requested reference was inspected at commit
`0d1c4dfc56221ca39a4790d22e7a36bb2ce9c3ca`. It is a standalone userscript
that scans ChatGPT DOM messages, renders a narrow fixed rail, expands near the
right edge, scrolls a selected user message into view, and chooses an active
entry nearest a 40% viewport reading line. That observer-driven implementation
is unsuitable inside `mm-chat`, where the active branch and full message list
already exist as typed state.

The current `mm-chat` conversation root uses `scrollbar-overlay`, a global
`6px` low-contrast thumb, and no stable scrollbar gutter. Its recent-message
first rendering also means a navigation target can exist in state before its
DOM node mounts. The implementation therefore uses a visible native scrollbar
first, then a React navigation component backed by message IDs plus an explicit
progressive-render reveal handshake.

## 2026-07-21 — G14.1 draggable chat scrollbar

The conversation scroll root now uses a dedicated `chat-scrollbar` class with
native `overflow-y: scroll`, a stable gutter, a visible `12px` WebKit track,
and higher-contrast hover/active thumb states. The existing scroll-follow event
handlers, composer clearance, and message geometry remain on the same element.

Verification:

```text
focused scroll-follow tests                         1 file / 5 tests
frontend full tests                                 187 files / 901 tests
frontend lint / typecheck / format / build          passed
source-built frontend/backend                       healthy / healthy
deployed overflow / gutter / scrollbar width        scroll / stable / 12px
deployed viewport / content height                  820px / 16,648px
native thumb drag scrollTop                          0 -> 14,667
native track click scrollTop                         0 -> 717
```

The Windows Chrome 150 smoke manipulated only the native scrollbar in a
disposable browser profile. It did not select, edit, create, or delete any
conversation or mutate provider, file, browser-authoritative, or database
state.

## 2026-07-21 — G14.2 user-message navigation and edge jumps

A typed React navigator now derives bounded labels from every user message on
the active branch, including attachment-only fallbacks. The desktop rail stays
`44px` wide beside the native scrollbar, expands to `256px` on hover or keyboard
focus, exposes full accessible button names, and tracks the message nearest the
scroll root's 40% reading line through one requestAnimationFrame-bounded scroll
listener. It observes content resize without a global DOM mutation observer.

Each rendered `MessageItem` exposes a stable element ID. A click targets that
ID with bounded container-local scroll math. If an older message precedes the
current progressive render window, the pure reveal resolver expands the window
to that index before the layout effect completes the jump. Jump-to-top also
reveals the full prefix; jump-to-bottom restores live bottom-follow. The two
edge controls use one-click immediate movement, while question navigation keeps
the reference project's smooth positioning.

The first live pass showed that smooth edge jumps across a 15,828px range took
about 1.75 seconds, so those two controls were changed to immediate movement
before closure.

Verification:

```text
focused navigation/scroll/message tests             4 files / 12 tests
frontend full tests                                 189 files / 905 tests
frontend lint / typecheck / format / build          passed
source-built frontend/backend                       healthy / healthy
deployed collapsed / expanded rail width            44px / 256px
deployed user entries / active entries              26 / 1
selected entry                                      user #14 (`hi`)
selected ID / highlighted ID                        exact match
selected message center / reading line              0.400 / 0.400
jump top scrollTop                                   0
jump bottom scrollTop / distance                    15,828 / 0
```

The Chrome smoke used the existing conversation read-only and changed only
scroll position in a disposable profile. It made no chat, file, provider,
browser-authoritative, or database mutation and consumed no provider quota.

## 2026-07-21 — G14.3 per-item navigation title preview

The rail no longer expands to show every message title. It remains `44px` wide
and renders one pointer-transparent floating preview for only the hovered or
keyboard-focused message, top control, or bottom control. Item mouse leave,
blur, rail mouse leave, and list scroll clear the preview directly. Native
`title` attributes were removed so the browser cannot leave a delayed second
tooltip behind.

Verification:

```text
focused navigation tests                             2 files / 4 tests
frontend full tests                                 189 files / 905 tests
frontend lint / typecheck / format / build          passed
deployed frontend container                         healthy
deployed rail width before / hover / after          44px / 44px / 44px
deployed user entries                               26
preview count on first / second hover               1 / 1
preview count next frame after mouse leave          0
native title attributes                             0
top / bottom preview                                会话顶部 / 会话底部
```

The live proof used Windows Chrome 150 against `http://localhost:18080` and
the existing conversation read-only. It changed only pointer position and
created no chat, file, provider, browser-authoritative, or database state.
