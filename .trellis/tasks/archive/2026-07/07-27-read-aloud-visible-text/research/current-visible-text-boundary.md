# Current visible-text and hosted TTS boundary

Date: 2026-07-27

## Proven mismatch

- `MessageItem.handleToggleReadAloud` passes raw `message.content` directly to
  `synthesizeSpeech`.
- The visible answer is rendered through `MessageOutputRenderer` and
  `MarkdownRendererClient`, which interpret CommonMark/GFM, raw sanitized HTML,
  links, citations, generated files, diagrams, and other visual components.
- The hosted Go service verifies the submitted raw text against the owned
  persisted message, then passes that raw Markdown/HTML directly to the Voice
  provider. This preserves ownership but makes the provider speak headings,
  emphasis markers, HTML tag names, CSS attributes, and link destinations that
  the user never sees.
- The reproduced weather message contains a raw styled `<div>` plus Markdown
  links. The cached audio objects are valid MP3 files, so the mismatch is a text
  projection defect rather than corrupt audio.

## Existing reusable boundaries

- `visibleMessageContentRef` already owns the rendered message area for image
  export sizing, but it also contains attachments and non-answer UI. A dedicated
  answer-output ref can expose browser `innerText` without copying the Markdown
  parser into frontend TTS code.
- `getMessageOutputBlocks` is the rendering authority for ordered text,
  reasoning, search, and tool blocks.
- Hosted source ownership and stale-text checks live in
  `backend/internal/voicejobs/service.go` and must remain authoritative.
- The cache `text_sha256` can safely become the digest of the projected speech
  text. `CommitCachedSynthesis` re-resolves the source under lock, so it can
  recompute the same projection; existing raw-text cache rows then miss and are
  replaced without a schema migration.

## Projection approach

Use `github.com/yuin/goldmark` v1.8.4 with `extension.GFM` in the Go service.
Traverse the parsed AST rather than applying regular expressions:

- retain textual nodes, paragraph/list/table boundaries, inline code, and
  visible fenced-code contents;
- retain link labels but omit link destinations and Markdown punctuation;
- omit image destinations and non-visible formatting metadata;
- parse raw HTML fragments with `golang.org/x/net/html`, retaining text nodes
  while excluding tags, attributes, comments, `script`, and `style` content;
- normalize whitespace into stable speech paragraphs.

Goldmark is CommonMark compliant and its GFM extension covers the same broad
Markdown family as the frontend's `remark-gfm`. Exact UI widgets are not
reimplemented in Go; the hosted provider projects only the authoritative stored
answer content. Browser/local providers use the rendered answer container's
`innerText`, which is the exact visible browser projection.

## Edge decisions

- Visible code contents remain speakable because the user can see them; only
  invisible fences/language markers are removed.
- Visible citation labels such as `W1` remain; URL targets and hover-only text do
  not.
- Empty readable projection fails before provider I/O instead of synthesizing
  markup-only input.
- No provider configuration, cache schema, TTL/LRU, or audio playback lifecycle
  change is required.
