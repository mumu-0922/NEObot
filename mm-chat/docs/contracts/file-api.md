# Phase 6 File API Contract

## Purpose

This contract defines the Phase 6 server file API above the backend
`ObjectStore`. The current implementation wires these endpoints to Postgres
`files` metadata and either the local object store or the Phase 6.4 MinIO/S3
adapter, selected by `STORAGE_BACKEND`.

## Endpoints

```http
POST   /v1/files
POST   /v1/files/remote
GET    /v1/files/{fileId}
GET    /v1/files/{fileId}/content
DELETE /v1/files/{fileId}
```

## Upload Request

`POST /v1/files` accepts `multipart/form-data`:

| Field                   | Required | Notes                                                                                   |
| ----------------------- | -------- | --------------------------------------------------------------------------------------- |
| `file`                  | yes      | File bytes. Backend enforces `MAX_UPLOAD_BYTES`.                                        |
| `purpose`               | yes      | `chat`, `workspace`, `knowledge`, `image`, `audio`, or `export`.                        |
| `conversationId`        | no       | Optional upload metadata; message ownership is enforced later when linking by `fileId`. |
| `workspaceId`           | no       | Workspace-scoped file metadata.                                                         |
| `knowledgeCollectionId` | no       | RAG import grouping, later phase.                                                       |
| `clientFileId`          | no       | Optional frontend retry/correlation ID.                                                 |

## Remote Import Request

`POST /v1/files/remote` accepts bounded JSON and returns the same File Response
as multipart upload:

```json
{
  "url": "https://cdn.example.com/notes.txt",
  "purpose": "chat",
  "conversationId": "optional-conversation-id",
  "clientFileId": "optional-client-id"
}
```

The backend downloads the object before message creation, enforces the normal
upload limit, computes SHA-256, and stores it through the same actor-owned
ObjectStore and Postgres flow. Only public HTTPS is accepted. Embedded
credentials, localhost/private/link-local/mixed DNS answers, unsafe redirects,
non-identity encoding, non-2xx responses, timeouts, empty bodies, and oversized
bodies fail closed. The source URL is request-only and is not persisted in file
metadata.

## File Response

```ts
export interface FileRecord {
  id: EntityId;
  fileName: string;
  mimeType: string;
  size: number;
  sha256: string;
  purpose: "chat" | "workspace" | "knowledge" | "image" | "audio" | "export";
  createdAt: IsoDateTime;
  downloadUrl: string; // /v1/files/{id}/content only
}
```

Responses must not expose local paths, MinIO bucket names, object keys, or
presigned URLs in the MVP.

## Validation & Errors

| HTTP  | Code                   | When                                                                         |
| ----- | ---------------------- | ---------------------------------------------------------------------------- |
| `400` | `INVALID_FILE_ID`      | `fileId` is not a UUID.                                                      |
| `400` | `INVALID_MULTIPART`    | Upload body is malformed.                                                    |
| `400` | `FILE_REQUIRED`        | No file part was supplied.                                                   |
| `400` | `INVALID_FILE_PURPOSE` | Purpose is missing or unsupported.                                           |
| `413` | `FILE_TOO_LARGE`       | File exceeds `MAX_UPLOAD_BYTES`.                                             |
| `400` | `REMOTE_URL_INVALID` / `REMOTE_URL_BLOCKED` | Remote URL is malformed or not public HTTPS.                 |
| `502` | `REMOTE_FETCH_FAILED` / `REMOTE_UPSTREAM_STATUS` | Remote download or upstream response failed.          |
| `504` | `REMOTE_FETCH_TIMEOUT` | Remote download exceeded the bounded timeout.                                |
| `404` | `FILE_NOT_FOUND`       | Metadata row is absent or deleted.                                           |
| `409` | `FILE_IN_USE`          | A live Knowledge Document Version still binds the File.                      |
| `429` | `RATE_LIMITED`         | Redis rate-limit middleware blocked the request before upload/download work. |
| `503` | `DATABASE_REQUIRED`    | File metadata repository is unavailable.                                     |
| `503` | `STORAGE_REQUIRED`     | Object store is unavailable.                                                 |

## Persistence Flow

```text
request multipart
  -> validate size/purpose/MIME
  -> stream bytes through sha256 hasher
  -> ObjectStore.Put(serverGeneratedObjectKey)
  -> insert files metadata row with sha256, byte_size, object_key
  -> return FileRecord
```

Remote import performs the bounded SSRF-safe download first, then enters the
same `sha256 -> ObjectStore.Put -> files row` flow. A fetch failure creates no
object and no metadata row.

Rollback rule: if Postgres insert fails after object write, delete the object.
If object write fails, do not create the metadata row.

## Phase 6.2 Implementation Notes

- `POST /v1/files` writes bytes through `ObjectStore`, computes SHA-256 while
  streaming, inserts the Postgres `files` row, and deletes the object if the DB
  insert fails.
- `GET /v1/files/{fileId}` returns metadata only.
- `GET /v1/files/{fileId}/content` streams bytes through the backend gateway.
- `DELETE /v1/files/{fileId}` locks the caller-owned `files` row with
  `FOR UPDATE`, rejects live Knowledge Version bindings with `409 FILE_IN_USE`,
  and atomically soft-deletes metadata plus writes
  `file.object.delete.requested`. It then attempts physical object deletion;
  the durable event permits idempotent retry after storage failure.
- Ownership is request-scoped through the implemented Phase 13 authenticated
  user context. Development fallback remains available only when the configured
  auth mode permits it; hosted/required mode never falls back to a fixed user.
- MinIO/S3 is available through the same `ObjectStore` interface when
  `STORAGE_BACKEND=minio` or `STORAGE_BACKEND=s3`; the HTTP response contract
  stays unchanged.
- Phase 6.3 message linking uses only returned `id` values. It does not trust
  upload-time `conversationId` metadata for authorization and never exposes
  object keys through message responses.
