# Object Storage Contract

## Purpose

Phase 6.1 adds a backend object-storage boundary without exposing MinIO/S3 yet.
The first implementation stores bytes on the local filesystem so a single-server
MVP can upload/download files before MinIO is introduced.

## Backend Interface

```go
type ObjectStore interface {
    Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
    Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
    Delete(ctx context.Context, key string) error
}
```

`ObjectStore` is byte storage only. It does not own auth, database rows,
conversation permissions, upload limits, MIME allow-lists, or SHA-256 checks.
Those belong to the Phase 6.2 file service and Postgres `files` metadata row.

## Local Filesystem Backend

Environment:

```env
STORAGE_BACKEND=local
LOCAL_STORAGE_DIR=./data/files
MAX_UPLOAD_BYTES=26214400
```

Rules:

- Object keys are server-generated and slash-separated, for example
  `users/{userId}/files/{fileId}`.
- Raw filenames are display metadata only and must never become object keys.
- Keys must not be absolute, contain leading/trailing whitespace, contain `..`,
  contain `.` path segments, contain empty segments, use Windows
  backslashes, or contain drive-style colons.
- Writes use a temp file followed by rename so failed uploads do not leave a
  visible partial object.
- Local object metadata may store `contentType`; Postgres remains canonical for
  file metadata once Phase 6.2 lands.

## S3/MinIO Handoff

The later adapter should keep the same interface and replace only the backing
implementation:

```text
backend file service -> ObjectStore -> local | minio | s3
```

The browser must still fetch through backend endpoints such as
`GET /v1/files/{id}/content`; it must not receive bucket names, object keys, or
direct MinIO URLs in the MVP.

## Verification

Current Phase 6.1 tests cover:

- put/get/delete round trip
- metadata content type round trip
- unsafe key rejection
- size mismatch cleanup
- cancelled context cleanup

## Phase 6.2 Usage

Phase 6.2 now uses this interface from the file service: upload writes object
bytes first, inserts Postgres metadata after SHA-256 is known, and removes the
object if metadata insertion fails. Downloads and deletes always resolve the
private object key from Postgres; browser responses never expose that key.
