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
