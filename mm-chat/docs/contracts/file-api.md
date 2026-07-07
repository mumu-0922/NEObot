# Phase 6 File API Contract

## Purpose

This contract defines the server file API that will sit above the Phase 6.1
`ObjectStore`. Phase 6.1 implements the storage boundary only; Phase 6.2 will
wire these endpoints to Postgres `files` metadata and the object store.

## Endpoints

```http
POST   /v1/files
GET    /v1/files/{fileId}
GET    /v1/files/{fileId}/content
DELETE /v1/files/{fileId}
```

## Upload Request

`POST /v1/files` accepts `multipart/form-data`:

| Field | Required | Notes |
| --- | --- | --- |
| `file` | yes | File bytes. Backend enforces `MAX_UPLOAD_BYTES`. |
| `purpose` | yes | `chat`, `workspace`, `knowledge`, `image`, `audio`, or `export`. |
| `conversationId` | no | Required later when attaching to a chat message. |
| `workspaceId` | no | Workspace-scoped file metadata. |
| `knowledgeCollectionId` | no | RAG import grouping, later phase. |
| `clientFileId` | no | Optional frontend retry/correlation ID. |

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

| HTTP | Code | When |
| --- | --- | --- |
| `400` | `INVALID_FILE_ID` | `fileId` is not a UUID. |
| `400` | `INVALID_MULTIPART` | Upload body is malformed. |
| `400` | `FILE_REQUIRED` | No file part was supplied. |
| `400` | `INVALID_FILE_PURPOSE` | Purpose is missing or unsupported. |
| `413` | `FILE_TOO_LARGE` | File exceeds `MAX_UPLOAD_BYTES`. |
| `404` | `FILE_NOT_FOUND` | Metadata row is absent or deleted. |
| `503` | `DATABASE_REQUIRED` | File metadata repository is unavailable. |
| `503` | `STORAGE_REQUIRED` | Object store is unavailable. |

## Persistence Flow

```text
request multipart
  -> validate size/purpose/MIME
  -> stream bytes through sha256 hasher
  -> ObjectStore.Put(serverGeneratedObjectKey)
  -> insert files metadata row with sha256, byte_size, object_key
  -> return FileRecord
```

Rollback rule: if Postgres insert fails after object write, delete the object.
If object write fails, do not create the metadata row.
