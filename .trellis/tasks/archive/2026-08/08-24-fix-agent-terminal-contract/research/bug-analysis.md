# Bug Analysis: Agent Terminal timeout and exit-status drift

## 1. Root Cause Category

- **Category**: B — Cross-Layer Contract
- **Specific cause**: The Provider schema used `RunTimeout` as the maximum
  explicit Terminal timeout while foreground execution validated the same
  argument against `CallTimeout`. Separately, a successfully returned executor
  result was treated as Tool success without classifying its shell exit code.

## 2. Why Earlier Behavior Failed

1. The model could legally emit `timeoutSeconds=40` under the advertised
   schema, but the 30-second foreground validator rejected it before spawning a
   process, consuming Tool rounds without useful work.
2. `/bin/bash` successfully returned the result of a missing `python` command,
   so transport completion hid exit code 127 and the UI showed a success state.
3. Later fallback searches and commands exhausted the eight-round budget before
   the requested workbook could be generated and published.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Use `CallTimeout` as the single explicit schema/executor authority | DONE |
| P0 | Runtime | Convert nonzero/timeout results to typed Tool failures while retaining bounded diagnostics | DONE |
| P0 | Test coverage | Pin schema max, background-null default, exit 127, timeout, event status, and UI label | DONE |
| P1 | Documentation | Record foreground/background timeout and evidence rules in executable contracts | DONE |
| P1 | Prompt guidance | Direct long work to background Jobs and avoid assuming a `python` alias | DONE |

## 4. Systematic Expansion

- **Similar issues**: Any LLM-visible numeric schema whose executor varies by
  mode can drift in the same way; check schema, default, and runtime bounds as
  one producer-to-validator path.
- **Design improvement**: Keep failure category independent from Tool transport
  so bounded output can return to the model without projecting success.
- **Process improvement**: Cross-layer tests must assert the Provider Result,
  durable ProcessStep, and rendered status for the same failed execution.

## 5. Knowledge Capture

- [x] Updated `.trellis/spec/backend/agent-runtime.md`.
- [x] Updated `.trellis/spec/backend/chat-tool-loop.md` with executable timeout
      and failure contracts.
- [x] Updated `.trellis/spec/guides/cross-layer-thinking-guide.md` with a
      schema-versus-executor checklist.
- [x] Updated product contract and deployment documentation.
