# Chat Components

Chat components render the main conversation experience: message input, message display, follow-up prompts, audio playback, attachments, and message actions.

## Files

- `MessageInput.tsx` handles text entry, file attachments, voice input, skill/plugin controls, model-aware controls, and send actions.
- `MessageItem.tsx` renders a single message with editing, copying, branching, deletion, playback, reading mode, and metadata controls.
- `ChatMessageNavigator.tsx` renders the desktop user-message rail, active reading marker, and top/bottom controls.
- `ChatGenerationProgress.tsx` renders the compact in-thread knowledge, web, or model generation status.
- `FollowUpQuestions.tsx` renders suggested next questions after a response.
- `AudioPlayer.tsx` renders audio playback controls for generated or attached audio.

## Scroll contract

- The main conversation root uses `chat-scrollbar`: it must keep a visible
  native scrollbar and stable gutter so mouse track clicks and thumb dragging
  remain available without replacing the existing scroll-follow handlers.
- User-message navigation stays outside the scroll root and reads stable message
  IDs. Top pauses live follow, bottom resumes it, and old targets must reveal
  through the progressive render contract before scrolling.
- The desktop navigation rail stays fixed-width. A message or edge-control title
  may appear only for the currently hovered or keyboard-focused item and must
  disappear immediately on mouse leave or blur; do not restore whole-rail
  expansion or delayed native `title` tooltips.

## Guidelines

- Keep chat-domain transformations in `src/lib/chat` or `src/lib/utils`.
- Keep API workflows in `src/services/api/chatService.ts`.
- Preserve accessibility for interactive message actions.
- Preserve IME-safe submit behavior and focus restoration in composer and reader flows.
- Avoid broad store subscriptions; select only the fields each component needs.
- Keep the final message above the floating composer, pause live bottom-follow on manual upward scrolling, and resume only when the reader returns to the bottom or sends a new prompt.
