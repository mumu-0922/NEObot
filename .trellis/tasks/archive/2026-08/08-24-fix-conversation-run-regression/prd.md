# Fix Conversation Run Regression

## Goal

Restore reliable message submission after the Pi-style Conversation Run rollout,
explain and repair the visible `Server generation failed` state left in an older
conversation, and prevent deployment/reconnect behavior from turning interrupted
but durable Runs into misleading permanent failures.

## What I already know

- The user reproduced the issue against the newly deployed Backend and Frontend.
- A new `New Chat` row shows a running spinner while the submitted text remains
  in the composer and no accepted user message is visible.
- An older Agent conversation contains a generated XLSX attachment followed by
  `Server generation failed`, so useful durable output and a generic terminal
  error are being shown together.
- Backend and Frontend were recreated during the preceding rollout; any Run that
  was active on the old in-memory Backend process lost its replay registry.
- The current design intentionally detaches Run lifetime from the browser SSE,
  but active-run replay authority is still process-local.
- Runtime evidence shows the new weather Run was accepted at `12:11:32` and
  completed successfully after 52.8 seconds at `12:12:25`.
- `MessageInput` keeps its submission lock until the `onSend` Promise settles;
  server `onSend` currently settles only after the whole stream. Switching to a
  sibling Conversation therefore leaves the shared composer unable to submit
  until the first Conversation finishes.
- The older failed assistant has durable code `AGENT_VERIFICATION_REQUIRED`,
  one retained attachment, and no durable error message.
- Its event chain used rounds 1–7 for Search/Terminal and round 8 for a
  successful `publish_file`. The local `MaxRounds=8` fence rejected the next
  round before the model could read/verify the published result.

## Assumptions

- The screenshots represent a regression or rollout-interruption path, not an
  intentional Provider failure.
- Existing completed content or generated attachments must never be deleted by
  the repair.
- The fix must remain one active Run per Conversation and allow sibling
  Conversations to run concurrently.

## Requirements

- Determine whether the new send is actively progressing, blocked in admission,
  or stranded before `message.started`.
- Make accepted user turns appear and clear the composer promptly.
- Release the shared composer submission lock as soon as the user message is
  durably accepted, while the Conversation Run continues in the background.
- Do not render a generic failure over a durable completed answer/attachment.
- Permit a small, bounded verification-only grace budget after an ordinary
  Tool-round limit when a successful mutation still requires evidence; retain
  fail-closed terminal failure if verification still does not complete.
- Map durable `AGENT_VERIFICATION_REQUIRED` codes to truthful localized copy
  after reload instead of `Server generation failed`.
- Reconcile Backend restart or missing replay state to durable message truth.
- Preserve exact per-Conversation cancellation and sibling concurrency.
- Repair only stale/inconsistent generation state; preserve user content and
  files.

## Acceptance Criteria

- [x] A fresh message becomes visibly accepted and the composer clears.
- [x] After Conversation A accepts its turn, switching to B allows B to submit
      while A continues running.
- [x] A normal Run completes without `Server generation failed`.
- [x] A completed durable attachment is not accompanied by a contradictory
      generic generation failure; unverified output remains visibly marked as
      unverified with a specific explanation.
- [x] A mutation on the last normal Tool round receives bounded evidence and
      `verify_completion` opportunities without weakening fail-closed policy.
- [x] Backend restart/reconnect reaches completed, failed, or explicitly
      interrupted durable truth without a permanent spinner.
- [x] Existing one-Run-per-Conversation and sibling concurrency tests remain
      green.

## Definition of Done

- Root cause is supported by runtime logs and source tracing.
- Focused Frontend/Backend regression tests pass.
- Proportional format, lint, typecheck, build, and cross-layer checks pass.
- Backend/Frontend are redeployed only if required; previous images remain
  available for rollback.
- The fix is committed, archived, and recorded in the Trellis journal.

## Out of Scope

- Replacing the single-server in-memory Run registry with a distributed queue.
- Deleting or rewriting valid historical user messages or generated files.
- Sending additional paid Provider requests without necessity.
- Treating unverified mutations as successful.

## Technical Notes

- Primary Frontend paths: `ChatApp.tsx`, `chatStore.ts`, and the server SSE
  client in `services/api/client/server/chatApi.ts`.
- Primary Backend paths: `handler.go`, `active_runs.go`, durable finalization,
  and Run replay/cancel handlers.
