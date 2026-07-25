# Direct Chat Attachment Context Design

## Runtime flow

```text
MessageInput raw file
  -> POST /v1/files
  -> POST /v1/chat/conversations/{id}/messages (fileId link)
  -> POST /v1/chat/conversations/{id}/stream
  -> fileProviderAttachmentResolver reads bounded bytes
     -> image/*: existing ProviderAttachment multimodal path
     -> supported document: synchronous bounded text extraction
  -> escaped <file name="..." type="..."> block in current user prompt
  -> provider request
```

The extracted body is request-derived context only. It is not persisted into
the visible message body and does not create embeddings, chunks, indexes, or a
retrieval lifecycle.

## Supported direct formats

- UTF-8 text, Markdown, CSV/TSV, JSON/JSONL, XML/HTML, YAML, SQL, logs, and
  common source-code extensions.
- PDF text through pinned pure-Go `github.com/ledongthuc/pdf`.
- DOCX, PPTX, and XLSX through bounded ZIP/XML traversal in the Go backend.
- Images remain native provider multimodal attachments.

Legacy DOC/PPT/XLS, unknown binary formats, invalid UTF-8, empty documents,
invalid archives, and over-boundary documents fail with actionable attachment
error codes before the provider is called.

## Limits

- Source bytes per direct document: 20 MiB.
- Extracted body per file: 60,000 context characters with a visible truncation
  notice.
- Combined document blocks: 160,000 context characters.
- PDF page count: 200.
- Office related-XML entry/total uncompressed limits: 8 MiB / 16 MiB; archive
  entry count: 4,096. Embedded media is not added to the XML expansion total.

## Trust boundary and known risk

Document bytes and text are untrusted user data. File/MIME attributes and body
delimiters are escaped, and a server-owned system instruction forbids treating
content inside `<file>` blocks as system/developer instructions. The ZIP/XML
path has explicit decompression limits; PDF input/page limits and panic
recovery bound common failure modes.

Known residual risk: the third-party PDF library performs some page-stream
decompression internally, so a deliberately adversarial compressed PDF has a
larger CPU/memory attack surface than plain text or the bounded Office parser.
The 20 MiB input and 200-page limits reduce but do not eliminate this risk. A
future production parser sandbox may replace the local PDF extractor after the
RAG sidecar exposes a stable successful parse contract; chat must not bypass
the currently closed Child-internal parser boundary.

## Send acceptance and rollback

The composer clears optimistically but restores its exact text/attachment
snapshot when the message has not been durably accepted. The store acknowledges
acceptance immediately after the user-message POST succeeds, so a later stream
failure does not restore a duplicate draft.

Rollback images captured before live rebuild:

- `mm-chat/backend:rollback-20260723-attachment-fix`
- `mm-chat/frontend:rollback-20260723-attachment-fix`
- `mm-chat/backend:rollback-20260723-before-stream-eof` restores the deployed
  20 MiB attachment build before `finish_reason` EOF compatibility hardening.

No storage migration or persisted message-body transformation is involved.

## PDF stream incident replay

The 2026-07-23 14:56 PDF report was not an upload or extraction failure. The
stored assistant body contained document-specific facts but stopped after 206
characters in the middle of `<div style="padding:10`, and the provider stream
had no accepted terminal marker. Replaying the same stored user message,
attachment, and `gpt-5.6-sol` model completed in 10.5 seconds with 424 deltas.

OpenAI-compatible parsing now accepts either `[DONE]` or a non-empty
`finish_reason` followed by clean EOF. It still rejects a clean EOF after only
partial deltas and no terminal marker, so genuinely interrupted output is not
silently promoted to completed.
