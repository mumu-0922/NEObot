# Attachment Processing Patterns

## Finding

Mainstream AI chat/document products use two complementary paths rather than
treating an uploaded file as message metadata only.

## Direct document context

This path is used for small or one-off documents that should affect the current
model turn immediately.

- OpenAI accepts file inputs. PDF processing exposes extracted text and page
  imagery; non-PDF documents are text-extracted; spreadsheets use a structured
  processing path.
- Anthropic allows uploaded `file_id` values to be referenced as document
  blocks. PDFs combine visual pages with extracted text; plain text is accepted
  directly, while Office formats generally need conversion or extraction.
- Gemini accepts uploaded files as document parts. PDF has native document
  understanding, while non-PDF documents are treated primarily as text.

Common properties:

- Parsing happens at a trusted server/provider boundary, not by blindly
  concatenating browser base64.
- The model receives a typed document block or a clearly delimited extracted
  text block.
- File type, size, token budget, parse status, and failure are explicit.

## Retrieval path

This path is used for large files, reusable files, or multi-turn document
question answering.

1. Parse and normalize the file.
2. Split it into provenance-preserving chunks.
3. Build embedding and/or lexical indexes.
4. Retrieve and optionally rerank relevant chunks for each question.
5. Inject only bounded evidence into model context.
6. Return citations to file/page/chunk locations.

This avoids sending an entire document on every turn and gives predictable
context cost, but indexing introduces latency and additional failure states.

## Mapping to Neo Chat

Neo Chat has parser source under
`mm-chat/rag/src/mm_chat_rag/offline_parser/native/`, but the live worker keeps
native parse output Child-internal and its sidecar success path does not yet
publish `canonical-ir.v2`. Directly importing those Python modules from chat
would bypass the worker's sandbox and contract boundary. The lowest-risk
immediate design is therefore:

- retain provider-native multimodal handling for images;
- perform bounded extraction in the Go chat backend for the explicitly
  supported direct-context formats;
- inject bounded, untrusted document context for small one-off files;
- reject unsupported or over-boundary inputs with an actionable error rather
  than silently calling the model with no document body.

The user explicitly chose not to add RAG, embeddings, indexing, retrieval, or
a paging tool for ordinary chat attachments. Unusually large/reusable document
workflows remain outside this task.

## Security and reliability constraints

- Extracted file content is untrusted data and must not be inserted as system
  or developer instructions.
- Apply MIME/extension validation, decompression limits, parser timeouts,
  maximum pages/rows/chars/tokens, and deterministic truncation.
- Keep parsing idempotent so retries do not duplicate indexes or associations.
- Preserve file/page/sheet/slide/chunk provenance for citations and debugging.
- A failed or incomplete parse must be visible; do not silently degrade to a
  text-only model request when the user's prompt depends on the attachment.

## References

- OpenAI file inputs:
  <https://developers.openai.com/api/docs/guides/file-inputs>
- OpenAI file search:
  <https://developers.openai.com/api/docs/guides/tools-file-search>
- Anthropic Files API:
  <https://platform.claude.com/docs/en/build-with-claude/files>
- Anthropic PDF support:
  <https://platform.claude.com/docs/en/build-with-claude/pdf-support>
- Gemini document processing:
  <https://ai.google.dev/gemini-api/docs/document-processing>
