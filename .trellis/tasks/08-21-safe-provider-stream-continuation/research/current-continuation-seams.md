# Current continuation seams

## Runtime facts

- `PROVIDER_STREAM_INTERRUPTED` is emitted only for typed Provider stream-read
  or incomplete-stream failures. The Backend finalizes the Assistant message
  as `failed` while preserving the accumulated partial content and durable
  Agent events.
- The browser-to-Backend SSE already has cursor replay for transport loss. This
  task therefore addresses an upstream Provider stream ending, not a browser
  reconnect.
- Regenerate creates a new Assistant sibling under the original User message
  and reruns the normal stream handler. In Agent mode that can execute Tools
  again.
- Provider conversation assembly can anchor at an Assistant message, preserving
  the exact branch and partial answer, then accept a synthetic User continuation
  instruction without persisting a fake visible User message.
- Agent Tool presentations are sanitized before persistence. They can provide
  bounded untrusted evidence to the continuation Provider request without
  recovering or persisting raw Tool payloads.

## Recommended seam

Use the existing stream endpoint and frontend stream state machine with an
explicit `continuationOfMessageId`. Validate the source on the Backend, create a
new sibling Assistant, bypass Tool/Search/RAG/Memory preparation, and call the
plain Provider stream directly. This preserves SSE reconnect, cancellation,
branch navigation, DTO normalization, and generation-error handling.

## Rejected approaches

1. **Mutate and reopen the failed Assistant** — breaks immutable Turn/event
   history and makes terminal events non-authoritative.
2. **Send a normal visible “continue” User message** — Agent mode could admit
   Tools again and pollutes the conversation with implementation prompts.
3. **Client-only disable flag** — Generic request config can be forged and does
   not physically remove Backend Tool runtimes.
4. **Automatic full retry** — repeats side effects and hides Provider cost.

## Security and failure boundaries

- Treat durable presentation evidence as untrusted data, not instructions.
- Use only the exact interruption code and same-parent sibling relation.
- Deny unresolved or outcome-unknown Tool states.
- Keep evidence and instruction bounded; never insert raw event payloads,
  credentials, host paths, or unbounded Terminal output.
- If continuation interrupts again, its newly preserved combined partial answer
  becomes a new eligible source; completed Tool history is still not replayed.
