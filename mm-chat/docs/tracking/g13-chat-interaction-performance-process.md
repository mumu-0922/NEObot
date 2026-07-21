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
