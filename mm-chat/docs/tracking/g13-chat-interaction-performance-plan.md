# G13 Chat Interaction Performance Plan

Status: complete. Work was split into independently reversible slices; each
slice must pass focused tests, the full frontend gate, a container rebuild, and
a deployed browser smoke before commit.

## Slice sequence

### [x] G13.1 — Image preview zoom

- Replace coarse wheel jumps and the `8x` ceiling with fine-grained continuous
  zoom up to a bounded practical `32x`.
- Disable inertial/zoom animations that compete with direct pointer input.
- Remove full-screen blur and large-image filter effects from the transform
  path while preserving the existing preview controls and focus trap.

### [x] G13.2 — Long-conversation scrolling

- Profile the existing heavy owner conversation without changing its data.
- Remove nested fixed-size `content-visibility` containment that changes the
  scroll range as long messages approach the viewport.
- Lazy-load embedded visual documents and remove the floating composer's
  backdrop blur from the scrolling paint path.

### [x] G13.3 — Conversation switching

- Add a small bounded in-memory cache for recently loaded server conversations.
- Render a cached conversation immediately, then revalidate against the Go API;
  Postgres remains authoritative and browser storage does not regain ownership.
- Guard asynchronous loads so an older response cannot overwrite a newer
  selection, and invalidate/update cache entries after conversation mutation.
- Render the recent message tail first and reveal older messages in idle
  batches while preserving the current scroll anchor.

### [x] G13.4 — Sidebar hover response

- Follow the proven `sub2api` navigation boundary: expanded rows already have
  labels and therefore must not render duplicate navigation tooltips.
- Remove color/background transitions from latency-sensitive primary rows so
  hover follows the current pointer target without visual interpolation.
- Keep collapsed navigation accessible with an immediate, solid tooltip while
  preserving the animated glass default everywhere else.
- Verify rapid cross-row movement in the deployed Windows Chrome runtime.

### [x] G13.5 — Conversation-list hover response

- Remove the remaining color/background interpolation from the shared session
  row renderer used by root and workspace conversations.
- Reveal each row's more-actions control on the current pointer target without
  an opacity delay.
- Preserve structural section/sidebar animation; only direct pointer feedback
  becomes immediate.
- Compare the same deployed conversation row before and after the change in
  Windows Chrome.

## Rollback

Each slice has its own commit. Revert only the affected frontend commit; there
is no database migration, persistent cache, or server-data rewrite.
