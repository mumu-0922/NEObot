# Cherry Studio and Kelivo Attachment Pipelines

Research date: 2026-07-23

Pinned revisions:

- Cherry Studio: `9fc545e7dabf9c17d4c5dd3dd27704b2bfa4c108`
- Kelivo: `545f7d67de250283232c9487aa5f4f42e85a7643`

## Cherry Studio

### Send and persistence

The composer permits attachment-only sends. At send time,
`buildFilePartsForAttachments` copies each selected file into Cherry's internal
file store, obtains a stable `fileEntryId`, and emits a `FileUIPart` containing
the filename, MIME type, physical `file://` snapshot, and Cherry metadata.

Relevant files:

- `src/renderer/components/composer/variants/ChatComposer.tsx`
- `src/renderer/utils/file/buildFileParts.ts`

### Provider-bound routing

Before calling the model, `prepareChatMessages` classifies every first-party
attachment using both its type and the target provider/model capability.

- Vision image -> native file part.
- Non-vision image -> OCR text.
- PDF on a supported first-party OpenAI/Anthropic/Gemini-style provider/model
  -> native PDF part.
- PDF on other endpoints -> locally extracted text.
- Office and text/code -> locally extracted text.
- Native audio/video -> native file part; unsupported media or binary -> an
  explicit model-visible note.

Native files are currently materialized as inline base64 `data:` URLs. Cherry's
own large-file port document says provider Files API upload is not yet wired on
the current main revision.

Non-native extraction uses:

- PDF: `pdf-parse` through `extractPdfText`.
- `.doc`: `word-extractor`.
- DOCX/PPTX/XLSX/XLS/ODF: `officeparser`.
- text/code: encoding-aware decode.
- extensionless: text/binary detection before decode.

Extraction is cached for 30 minutes using file entry version `(mtime, size)`.

Relevant files:

- `docs/references/ai/chat-attachments.md`
- `src/main/ai/messages/attachmentRouting.ts`
- `src/main/ai/messages/attachmentTextExtraction.ts`
- `src/main/ai/messages/fileProcessor.ts`
- `src/main/ai/runtime/aiSdk/params/nativeFileSupport.ts`

### Context guard and overflow

Extracted content is always made visible to the model immediately. It is capped
at 8,000 characters per file. For tool-capable models, the truncation trailer
tells the model to call `read_file(filename, offset)` to page the remainder.
The tool resolves the filename against a per-request attachment allow-list;
internal file IDs never reach the model.

If extraction/materialization fails, Cherry inserts an explicit
`[could not read this file]`-style note rather than silently dropping the
attachment or aborting the entire request.

This is not RAG: the current chat attachment path is eager bounded inline text
plus sequential paging. Cherry has separate knowledge-base functionality, but
normal chat attachments are not automatically embedded or retrieved.

## Kelivo

### Selection and persistence

The Flutter file picker accepts images, common audio/video, PDF, DOCX, and a
specific set of text/code extensions. Selected files are copied into the app's
upload directory. The persisted user message embeds local reference markers:

```text
[image:/local/path]
[file:/local/path|filename|mime]
```

Attachment-only sends are explicitly allowed because the send guard rejects
only when text, image paths, and documents are all empty.

Relevant files:

- `lib/features/home/services/file_upload_service.dart`
- `lib/utils/file_import_helper.dart`
- `lib/features/home/services/message_generation_service.dart`
- `lib/features/home/controllers/home_view_model.dart`

### Model-bound transformation

When preparing retained conversation messages for an API request, Kelivo
re-parses the markers. For every retained user document it extracts text in a
background Dart isolate and prepends it to that user message as:

````text
## user sent a file: <filename>
<content>
```
<full extracted text>
```
</content>
<user text>
````

Extraction uses:

- PDF: Syncfusion `PdfTextExtractor` (text only).
- DOCX: ZIP + `word/document.xml` traversal.
- DOC: explicitly unsupported.
- Everything else: UTF-8 decode with malformed bytes allowed.

The extracted text cache is keyed by local path and validated with modification
time plus size. A processing indicator is displayed around message preparation.

Images follow a separate path: without OCR they are inlined for multimodal API
handling; with OCR enabled, OCR text is injected instead. Some audio/video are
passed only when the provider/model-specific path supports them.

Relevant files:

- `lib/core/services/chat/document_text_extractor.dart`
- `lib/features/home/services/message_builder_service.dart`
- `lib/features/home/services/message_generation_service.dart`
- `lib/features/home/widgets/file_processing_indicator.dart`

### Limits and failure behavior

Kelivo limits retained history by message count before document extraction, not
by extracted file size or token count. In the inspected revision there is no
per-document extraction cap, `read_file` paging, automatic RAG path, or
provider-native PDF route. Consequently a large file can expand the request
substantially after context trimming.

Extractor failures are generally converted into diagnostic strings such as
`[[Failed to read PDF: ...]]`, which are then included in the model-visible file
block. This avoids silent absence, but exposes lower-level failure text and is
less controlled than Cherry's sanitized note behavior.

## Comparison

| Concern | Cherry Studio | Kelivo |
|---|---|---|
| Attachment-only send | Supported | Supported |
| File persistence | Internal file entry + stable ID | Copied local file + path marker |
| Native modality routing | Provider/model-aware image, PDF, audio, video | Images/media have separate API path; docs become text |
| PDF | Native when supported, else text extraction | Always local text extraction |
| Office | Broad office parser support | DOCX only; DOC unsupported |
| Text context | 8k chars/file inline | Entire extracted body inline |
| Large-file continuation | `read_file` paging | None |
| Automatic RAG | No, not for ordinary chat attachments | No |
| Parse cache | File-entry version, 30 min | Local path + mtime + size |
| Failure behavior | Sanitized visible note | Diagnostic text included in file block |

## Implication for Neo Chat

For the immediate regression, Cherry's routing invariant is the strongest fit:
every attachment must be either a provider-native typed part or explicit,
bounded extracted text; the model must never see only an opaque file ID.

A pragmatic Neo Chat sequence is:

1. Allow attachment-only messages and clear composer state only after durable
   message acceptance.
2. Add server-side provider/model capability routing.
3. Keep native images as typed file parts; do not send PDFs through the current
   image-only provider attachment contracts.
4. Parse supported documents in a bounded Go backend extraction path and
   inline the resulting excerpt. The current RAG worker parser success output
   is production-closed and must not be bypassed.
5. Surface parse/read failure explicitly.
6. Reject or visibly truncate overflow. The user explicitly excluded automatic
   RAG and `read_file` paging from ordinary chat attachments.

Kelivo's direct prompt injection is useful as a minimal reference but should not
be copied without a file-size/token cap, sanitized failures, and a stronger
untrusted-document boundary.
