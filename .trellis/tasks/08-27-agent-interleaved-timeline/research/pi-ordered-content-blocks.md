# Pi Ordered Assistant Content Blocks

## Question

How does Pi preserve explanatory text, thinking, and Tool execution in a
readable chronological conversation, and what should Neo Chat reuse?

## Evidence

- `/home/mumu/projects/pi-web/lib/types.ts` defines one ordered
  `AssistantContentBlock[]` union containing `text`, `thinking`, `image`, and
  `toolCall` blocks.
- `/home/mumu/projects/pi-web/components/MessageView.tsx` maps those blocks in
  their original array order. A Tool result is looked up by `toolCallId` and
  rendered beneath its Tool call rather than appended to another section.
- `/home/mumu/projects/pi-web/lib/message-display.ts` identifies only the
  trailing Text/Image blocks after the last process block as the final answer;
  earlier Text blocks remain part of process history.
- `/home/mumu/projects/pi-web/components/ChatWindow.tsx` may collapse the
  process group visually, but it does not reorder the content blocks inside
  that group.

## Mapping To Neo Chat

- Neo Chat already has a stronger durable source than Pi's client array:
  immutable `chat_agent_events` with a per-Turn sequence.
- Neo Chat's current event projector covers Context, Reasoning, and Tool but no
  ordinary user-visible text block. The final `message.content` is consequently
  rendered after the transcript regardless of the Provider round in which text
  was produced.
- The correct adaptation is not to copy Pi's storage shape or infer ordering in
  React. Add an explicit narration block to Neo Chat's existing Agent event
  protocol and preserve its durable sequence.

## Feasible Approaches

### A. Durable narration blocks (recommended)

Classify Provider text at the Tool-round boundary, persist Tool-bearing round
text as narration events, and retain only the final no-Tool round in
`message.content`.

- Exact live/reload parity.
- No heuristic splitting.
- Preserves typed Tool presenters and current event storage.
- Requires Backend and Frontend contract changes.

### B. Frontend-only sentence splitting

Split `message.content` around Tool timestamps during rendering.

- Small code diff.
- Cannot prove sentence-to-Tool ownership, diverges on reload, and can duplicate
  or move text. Rejected.

### C. Synthetic progress labels

Generate prose from Tool names/statuses.

- Always produces an apparent explanation.
- Can fabricate intent or conclusions and duplicates typed Tool labels.
  Rejected as the primary authority; typed Tool cards remain only a fallback
  when no narration exists.

## Conclusion

Use approach A: a single sequence-authoritative transcript with explicit
narration blocks, following Pi's ordered-block principle without replacing Neo
Chat's durable event model.
