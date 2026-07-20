# G11.12 Stream Scroll Follow and Composer Clearance

## 2026-07-20 — Manual-scroll and overlay closure

### Outcome

Streaming chat now follows the newest output only while the reader remains at
the bottom. An upward wheel, scrollbar drag, or pointer/touch scroll pauses
automatic following immediately. Scrolling back to the bottom resumes it. The
message viewport also reserves measured composer clearance, so the last output
line remains visible above the floating input.

### Root cause

The old implementation stored only a broad `distanceFromBottom < 160` flag and
called smooth `scrollIntoView` after every streamed message update. Repeated
smooth-scroll requests fought manual wheel input. Programmatic `scrollTop`
changes caused by the welcome transition or layout growth could also be
misclassified as user scrolling. Separately, the message viewport reserved a
fixed `8rem`, which could be shorter than the rendered composer.

### Implementation

- `src/lib/chat/scrollFollow.ts` owns bottom-distance, pause/resume, and
  composer-clearance calculations.
- Upward wheel intent pauses follow before the subsequent scroll event.
- Pointer/touch intent gates upward-scroll classification, so layout shifts do
  not disable follow.
- Returning within 48 pixels of the bottom resumes follow.
- Streaming uses direct container-bottom alignment rather than stacking smooth
  `scrollIntoView` animations.
- `ResizeObserver` measures the composer wrapper and reserves its height plus a
  gap, with a safe first-render minimum.
- Sending a new prompt or switching conversations restores bottom following.

### Verification

| Gate                    | Result                                           |
| ----------------------- | ------------------------------------------------ |
| focused scroll tests    | 4 passed                                         |
| lint / typecheck        | passed                                           |
| production Docker build | passed                                           |
| initial live follow     | bottom distance `0`                              |
| live manual pause       | content `121 -> 150`; scrollTop stayed `77`      |
| live resume             | content `597 -> 621`; bottom distance stayed `0` |
| composer clearance      | last bubble ended about 69 px above composer     |
| Compose health          | backend and frontend healthy                     |
| smoke cleanup           | all temporary scroll conversations deleted       |

### Boundary and rollback

The browser still controls native wheel/touch momentum. The app only decides
whether new streamed content may move the message viewport. Nested scrollable
content can pause follow, which is preferable to pulling the reader away from
the content they are inspecting.

Rollback reverts the helper, ChatApp wiring, tests, and documentation. No API,
schema, provider, message persistence, or secret changed.
