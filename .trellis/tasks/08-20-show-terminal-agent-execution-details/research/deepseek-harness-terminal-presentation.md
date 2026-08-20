# DeepSeek Harness Terminal presentation research

## Source

- Repository: <https://github.com/deepseek-ai/deepseek-harness>
- Inspected commit: `141eb6fef83422698aef7a981029e843e8161534`
- Temporary read-only research checkout: `/tmp/deepseek-harness.9TPuce`

## Execution/event model

DeepSeek Harness records a durable two-stage sequence rather than reducing a
Tool to one generic trace row:

```text
assistant tool-call block
  -> session tool/call (logged before execution)
  -> pre-execute policy / approval / guards
  -> execute / post-execute / result finalization
  -> session tool/result
  -> UI completed card
```

The authoritative flow is documented in
`docs/tool-execution-pipeline.zh.md`. The event producer/consumer matrix in
`docs/event-producer-consumer.zh.md` shows that session events drive both live
rendering and replay.

## Tool-owned presentation

`packages/core/tools/src/presentation.ts` defines provider-neutral tagged
render intents. A Tool may return `presentCall(args)` and
`presentResult(args, result)`; the UI does not need a dictionary keyed by Tool
name.

For bash, `packages/shell/tool-bash/src/index.ts` does this:

- foreground `presentCall` returns `card: 'terminal'`, command title,
  description and optional cwd;
- foreground `presentResult` returns `card: 'terminal'`, captured output and
  parsed exit code/signal;
- background starts and execution errors use a generic card;
- presenters are pure and replay-safe.

`packages/client/ui-tool/src/client/tool/models/terminal-card-model.ts`
combines the call-side command/cwd with result-side output/exit state exactly
once. `ToolRow.tsx` keeps rows collapsed by default and renders the expandable
body through a `TerminalBlock`. The terminal body has its own height/scroll
boundary so verbose output cannot take over the conversation.

## Mapping to neo-chat

neo-chat already has the same lifecycle facts, but loses presentation data:

```text
ProviderToolExecutionEvent
  -> toolProcessTrace
  -> durable chat_agent_events + process.step.updated SSE
  -> ProcessTracePanel
```

Current Terminal execution deliberately retains only Tool name, mode, round,
classification, timeout and status. The live failing message
`101943cd-7c79-4160-9afa-13a75886757b` therefore contains two completed
Terminal steps whose only argument summary is `{"timeoutSeconds":30}`.

Recommended adaptation:

1. Add a tagged, typed `presentation.card = "terminal"` object to a
   `ProcessStep`; keep it separate from diagnostic `detail`.
2. Populate it at call time with bounded/redacted command and cwd, then at
   result time add exit/timed-out/truncated state.
3. Persist the same sanitized presentation in durable Agent events and stream
   it through the existing `process.step.updated` event, preserving live/reload
   parity.
4. Render a collapsed command summary plus an expandable terminal-like card in
   `ProcessTracePanel`.
5. Unlike DeepSeek Harness, do not persist raw stdout/stderr in this slice.
   neo-chat's existing contract treats Tool output and file content as
   sensitive process data; the assistant answer remains the place for the
   model's user-facing result.

This preserves the useful architectural lesson—Tool-owned/typed presentation
separate from diagnostics—without widening raw output retention.
