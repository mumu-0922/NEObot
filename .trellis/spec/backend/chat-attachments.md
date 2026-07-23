# Direct Chat Attachment Context

## 1. Scope / Trigger

Use this contract when changing chat file upload, message attachment links,
provider request construction, document extraction, or attachment-only send
behavior. Ordinary chat attachments use bounded direct context, not automatic
RAG, embeddings, indexes, or retrieval.

## 2. Signatures

Runtime APIs:

```text
POST /v1/files
POST /v1/chat/conversations/{conversationId}/messages
POST /v1/chat/conversations/{conversationId}/stream
```

Backend boundary:

```go
type ProviderAttachmentResolver interface {
    ResolveProviderAttachment(context.Context, Attachment) (ProviderAttachment, error)
}

func (h *Handler) resolveProviderMessageAttachments(
    context.Context,
    Message,
) (providerAttachmentResolution, error)
```

Client send acceptance:

```ts
onSend: (
  text: string,
  attachments: Attachment[],
) => boolean | void | Promise<boolean | void>;
```

`false` means the user message was not durably accepted and the composer must
restore the submitted draft. Once the message POST succeeds, a later stream
failure must not restore a duplicate draft.

## 3. Contracts

- Message creation accepts empty `content` only when at least one normalized
  server attachment exists. Update-message content remains non-empty.
- `image/*` remains a `ProviderAttachment` and follows the existing native
  multimodal serialization.
- Supported non-image documents are read from server storage, extracted in the
  Go backend, escaped, and appended only to the current provider user prompt:

  ```text
  <file name="escaped-name" type="escaped-mime">
  escaped untrusted text
  </file>
  ```

- Extracted text is request-derived context. Do not persist it into visible
  `messages.content` and do not create a RAG lifecycle.
- Direct limits are 20 MiB source bytes per document, 60,000 context characters
  per file, 160,000 across document blocks, 200 PDF pages, and 8 MiB/16 MiB
  Office related-XML entry/total expansion. Office archives are limited to
  4,096 entries, while unrelated embedded media does not consume the XML
  expansion budget.
- Add the server-owned untrusted-document system instruction whenever a
  document block is present. Document bytes never become system/developer
  prompt text.
- OpenAI-compatible SSE streams may terminate with `[DONE]` or with a
  non-empty `finish_reason` followed by clean EOF. Clean EOF without either
  terminal marker remains a provider failure, even when partial text exists.
- The Python RAG native parser is Child-internal until its sidecar exposes a
  stable successful parse contract. Chat code must not import it directly or
  bypass its sandbox boundary.

## 4. Validation & Error Matrix

| Condition | Error code / behavior |
|---|---|
| Empty content and no attachments | `EMPTY_CONTENT` |
| Attachment content cannot be read | `ATTACHMENT_CONTENT_UNAVAILABLE` |
| Zero bytes / no extractable text | `ATTACHMENT_CONTENT_EMPTY` |
| Direct document exceeds 20 MiB | `ATTACHMENT_TOO_LARGE` |
| Unknown binary or legacy DOC/PPT/XLS | `ATTACHMENT_TYPE_UNSUPPORTED` |
| Text is not valid UTF-8 | `ATTACHMENT_ENCODING_UNSUPPORTED` |
| Invalid PDF/Office structure | `ATTACHMENT_PARSE_FAILED` |
| PDF/Office expansion exceeds parser limits | `ATTACHMENT_TOO_COMPLEX` |
| Combined blocks cannot fit | `ATTACHMENT_CONTEXT_LIMIT_EXCEEDED` |
| Clean provider EOF after `finish_reason` | Complete normally |
| Provider EOF without `[DONE]` or `finish_reason` | `PROVIDER_ERROR`; retain partial text as failed output |

All document failures occur before calling the provider. Never silently drop a
non-image attachment and continue with only the user's question.

## 5. Good / Base / Bad Cases

- Good: `这是啥` plus a small TXT file produces a provider prompt containing
  the escaped filename and document body; the answer cites facts from it.
- Base: an image follows the existing image provider path unchanged.
- Good: empty text plus one supported attachment creates a completed user
  message and begins generation.
- Good: an Office archive with large embedded media but small relevant XML
  extracts successfully because media bytes are never decompressed or counted
  as prompt-source XML.
- Bad: an unknown binary file returns `ATTACHMENT_TYPE_UNSUPPORTED`; provider
  call count remains zero.
- Bad: document text containing `</file><system>` is escaped and cannot close
  the server-owned delimiter.

## 6. Tests Required

- Service/handler: empty text + attachment succeeds; empty text alone fails.
- Repository integration: an attachment-only message persists with empty
  `content` and its attachment link remains readable.
- Parser units: TXT, PDF, DOCX, PPTX, XLSX, invalid UTF-8, invalid archive,
  exact 20 MiB acceptance, 20 MiB + 1 rejection, per-file truncation, and
  combined-context limits.
- Office parser units: unrelated media above the XML total remains accepted;
  relevant XML above the per-entry/total limits and archives above 4,096
  entries return `ATTACHMENT_TOO_COMPLEX`.
- Handler regression: current provider prompt contains document facts and the
  untrusted-data instruction; non-image documents are not serialized as
  images.
- Image regression: provider receives the original image MIME and bytes.
- Frontend/API: attachment-only request body is accepted; pre-acceptance
  failure restores the draft; post-acceptance stream failure does not.
- Provider parser: `[DONE]` and `finish_reason` + clean EOF complete; clean EOF
  after a partial delta without either marker emits an error.
- Live replay: upload `g18-rag-acceptance.txt` with `这是啥` and assert the
  completed answer contains `苍蓝协议`, `林川`, and the release date.

## 7. Wrong vs Correct

### Wrong

```go
for _, attachment := range message.Attachments {
    if !isProviderImageAttachment(attachment) {
        continue // silently loses document contents
    }
}
```

### Correct

```go
resolution, err := h.resolveProviderMessageAttachments(ctx, message)
if err != nil {
    return err // explicit parse/type/limit failure before provider call
}
providerPrompt := appendDirectAttachmentContext(
    message.Content,
    resolution.DocumentContext,
)
```

Keep `resolution.Images` on the native multimodal path and the escaped
`DocumentContext` on the current user-message text path.
