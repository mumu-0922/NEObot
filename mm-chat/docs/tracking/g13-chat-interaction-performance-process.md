# G13 Chat Interaction Performance Process

## 2026-07-21 — G13.1 image preview zoom

The preview combined a coarse `wheel.step` of `0.2`, an `8x` ceiling, a
full-screen `backdrop-blur-2xl`, a large-image `drop-shadow-2xl`, and transform
transitions. A normal wheel delta could jump directly toward the ceiling while
the filters enlarged the compositing cost of every transform.

The deployed preview now uses a `0.008` smooth wheel step, a bounded `32x`
maximum, direct panning with velocity/zoom animations disabled, and a promoted
transform layer without full-screen or image filters.

Verification:

```text
focused preview tests                              2 files / 6 tests
frontend full tests                                184 files / 888 tests
frontend lint / typecheck / format / build         passed
frontend/backend containers                        healthy / healthy
deployed real-image wheel events                   50 / 50 unique scales
deployed resulting scale                           13x (above old 8x cap)
deployed backdrop/image filter                     none / none
deployed long tasks during sample                   none
```

The existing conversation and image were used read-only. No chat, file,
provider configuration, or persistent browser data was modified.

## 2026-07-21 — G13.2 long-conversation scrolling

The deployed 51-message conversation exposed two nested fixed-size
`content-visibility` boundaries: the stream wrapper reserved `240px`, while
each `MessageItem` reserved `140px`. Several real assistant messages were more
than `1,400px` high. As skipped messages approached the viewport, their layout
height replaced the placeholder and repeatedly changed the scroll range.

The message stream now keeps its real height stable instead of applying nested
placeholder containment. Inline HTML visual iframes use native lazy loading,
and the floating composer keeps its current surface styling without a live
backdrop blur over moving messages.

Verification:

```text
focused rendering/scroll tests                      4 files / 24 tests
frontend full tests                                 185 files / 890 tests
frontend lint / typecheck / format / build          passed
frontend/backend containers                         healthy / healthy
deployed owner conversation                         51 rendered messages
old 100-frame scroll-height changes                 42 (11,964px -> 18,145px max)
new 100-frame scroll-height changes                 0 (stable 18,082px)
old/new layout count during probe                   49 / 1
new frame intervals above 20ms / long tasks         0 / 0
composer backdrop / inline iframe loading           none / lazy
```

The owner conversation was profiled read-only. No message, file, session
configuration, or server state was changed.

## 2026-07-21 — G13.3 conversation switching

Server conversation selection previously waited for every `/messages` request,
rebuilt the complete tree, and synchronously mounted every historical Markdown
message before the selected title and content could paint. The 51-message
conversation took about `231ms` to appear and produced a `183ms` maximum frame
gap even though its local backend response took only about `20ms`.

A six-entry least-recently-used memory cache now snapshots the immutable active
tree when leaving or loading a server conversation. Selection immediately uses
the snapshot, then always revalidates through Go/Postgres. An unchanged result
keeps the same tree reference and does not trigger a duplicate render; a
changed result replaces the snapshot. Cache entries are invalidated or updated
across message mutations and deletion, while the existing request generation
guard prevents stale reads from overwriting a newer selection.

Long conversations initially mount the eight most recent messages. Older
messages are prepended in six-message idle batches, and the scroll container is
adjusted by the added height so the visible last message stays fixed. This is
render scheduling only: the full server message tree remains available for
context, branches, edits, and generation from the first selection update.

Verification:

```text
focused cache/render/store tests                    4 files / 37 tests
frontend full tests                                 187 files / 897 tests
frontend lint / typecheck / format / build          passed
frontend/backend containers                         healthy / healthy
old 51-message title paint / max frame gap          231ms / 183ms
new cached 51-message title+tail paint              41-49ms
new cold 2-message title / full message paint       15-18ms / 23-25ms
background revalidation                             performed on every switch
progressive rendered counts                         8,14,20,...,50,51
last-message bottom during prepend                  236.5px-237.3px
memory cache bound / browser persistence            6 / none
```

The browser smoke switched only among existing owner conversations. It did not
send, edit, delete, or persist any message and did not expose provider secrets.
