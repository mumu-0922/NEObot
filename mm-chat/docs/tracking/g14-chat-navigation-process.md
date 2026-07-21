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
