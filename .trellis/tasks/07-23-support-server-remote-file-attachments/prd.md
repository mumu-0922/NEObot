# Support server remote file attachments

## Goal

Make the existing Remote File URL composer flow work in server mode by
importing the remote object into the server-owned file store before the user
message is accepted. The resulting attachment must use the same `fileId`
contract, persistence, parsing, and provider path as an ordinary upload.

## What I already know

* `RemoteFileModal` currently creates a URL-only `Attachment`.
* Server chat calls `uploadMessageAttachmentsForServer`, whose conversion
  deliberately rejects URL-only attachments.
* `POST /v1/files` already stores bounded multipart uploads in the current
  actor's object namespace and returns a `FileRecordDTO`.
* Direct chat attachments are server-owned, parsed in Go, and limited to 20
  MiB for document context. The file endpoint currently accepts up to 25 MiB.
* The backend already has a public-address-only dialing pattern in the Web
  Search client, but it is not exposed as a shared package.

## Requirements

* Add an authenticated server File API operation that accepts a public HTTPS
  URL plus the same purpose/conversation/client metadata as a normal upload.
* Validate the URL before dialing and again at the transport boundary:
  HTTPS only, no embedded credentials, no localhost/private/link-local/
  multicast/unspecified address, and no proxy use.
* Resolve DNS and reject the request if any returned address is non-public,
  preventing mixed-answer and DNS-rebinding bypasses.
* Follow only a small bounded number of redirects and revalidate every target.
* Request identity encoding, enforce connect/header/total timeouts, reject
  non-2xx responses, and bound the response body by the configured file upload
  limit before storage.
* Derive a safe display filename from Content-Disposition or the final URL and
  derive MIME from a valid response Content-Type or content sniffing.
* Store the imported bytes through the existing `files.Service.Upload` path so
  actor ownership, SHA-256, metadata, MinIO/object storage, and DB records stay
  identical to local uploads.
* Update the server frontend adapter so URL-only attachments call the remote
  import API and become server-backed attachments before message creation.
* Preserve the current direct/BYOK URL behavior outside server mode.
* Cancellation before durable message acceptance must cancel the import and
  preserve composer draft restoration behavior.

## Acceptance Criteria

* [ ] A public HTTPS text/image/audio URL imports and returns a normal server
      `fileId`, then sends through the existing attachment path.
* [ ] Localhost, literal private IP, DNS-to-private, embedded credentials,
      HTTP, unsafe redirect, excessive redirects, timeout, non-2xx, empty body,
      and oversized body fail explicitly without creating a file record.
* [ ] Content-Length above the limit fails before reading the body; chunked or
      dishonest responses are still stopped at limit + 1.
* [ ] Imported metadata includes purpose, conversationId, and clientFileId.
* [ ] Existing multipart upload, download, delete, local-mode adapters, native
      images, direct document parsing, and attachment-only sends remain green.
* [ ] Backend tests, Go vet, frontend tests, lint, typecheck, format check, and
      production build pass.

## Definition of Done

* Backend handler/fetcher and SSRF regression tests are added.
* Frontend File API/service tests cover the remote-to-server conversion.
* The attachment contract documents the server remote-import boundary.
* Rollback is removal of the remote endpoint/client method; multipart uploads
  and existing server-backed attachment contracts remain unchanged.

## Out of Scope

* Persisting signed remote URLs or provider credentials.
* Importing authenticated/private-network URLs.
* Adding RAG, embedding, or indexing for ordinary remote chat attachments.
* Background imports, resumable downloads, archive extraction, or more than
  the existing upload-size limit.

## Technical Approach

Use a backend-mediated import endpoint rather than browser fetch or passing the
URL directly to the model. The backend buffers at most `maxUploadBytes + 1`
to establish a trustworthy size, then invokes the existing upload service.
The frontend file service detects URL-only attachments and calls this endpoint;
the rest of the chat send flow remains unchanged.

## Decision (ADR-lite)

**Context**: Browser fetching is CORS-dependent and exposes user network
identity. Passing URLs directly to providers is inconsistent and bypasses the
server file/persistence contract.

**Decision**: Import public HTTPS resources in the Go backend with SSRF-safe
dialing and bounded buffering, then reuse normal server file storage.

**Consequences**: Remote attachment behavior becomes provider-independent and
auditable, at the cost of one bounded server download and temporary in-memory
buffer per import.

## Technical Notes

* Backend: `internal/files`, File handler/service, HTTP route metrics.
* Frontend: File API types/adapters and `uploadMessageAttachmentsForServer`.
* Applicable spec: `.trellis/spec/backend/chat-attachments.md`.
