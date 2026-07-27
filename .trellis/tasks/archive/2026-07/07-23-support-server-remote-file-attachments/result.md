# Server remote file attachment result

## Outcome

- `337dd19` added authenticated `POST /v1/files/remote`, SSRF-safe HTTPS
  fetching, bounded redirect/DNS/dial validation, identity encoding, response
  size enforcement, safe filename/MIME derivation, and ordinary File service
  persistence.
- Server-mode URL-only attachments now import to actor-owned `fileId` values
  before message acceptance. Local/BYOK behavior remains unchanged.
- File, frontend-client, and direct chat attachment contracts were updated.

## Verification

- Remote fetcher tests cover malformed/HTTP/credential-bearing URLs, literal
  and DNS-resolved private or mixed addresses, pinned dial addresses, unsafe
  redirects, redirect limits, non-2xx, empty bodies, timeouts, Content-Length,
  streamed limit + 1, identity encoding, metadata, and existing Upload reuse.
- Frontend tests cover remote-import DTOs, URL-only conversion, cancellation,
  server attachment normalization, and local-adapter preservation.
- `go vet ./...`, focused Go packages, and `18 files / 196` focused frontend
  tests passed. The later full standalone gate also passed the entire product
  tree.

## Live evidence — 2026-07-27

- Importing public `https://example.com/` returned HTTP 201 with a non-empty,
  actor-owned `chat` file record.
- Importing `https://127.0.0.1/private` returned HTTP 400
  `REMOTE_URL_BLOCKED` before persistence.
- The temporary public file was deleted with HTTP 204, and a subsequent read
  returned HTTP 404.

## Rollback

Revert `337dd19`. No schema migration is involved; existing multipart files and
message attachment records are unchanged.
