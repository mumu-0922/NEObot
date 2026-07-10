# Object Storage Contract

## Purpose

Phase 6 adds a backend object-storage boundary. Phase 6.1 introduced the local
filesystem fallback; Phase 6.4 adds a MinIO/S3-compatible adapter behind the
same interface.

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

## MinIO/S3 Backend

Environment:

```env
STORAGE_BACKEND=minio # local|minio|s3
S3_ENDPOINT=http://minio:9000
S3_BUCKET=neo-chat-files
S3_REGION=us-east-1
S3_ACCESS_KEY_ID=<app-scoped-access-key>
S3_SECRET_ACCESS_KEY=<secret>
S3_USE_SSL=false
S3_FORCE_PATH_STYLE=true
S3_BUCKET_AUTO_CREATE=false
MAX_UPLOAD_BYTES=26214400
```

Rules:

- `STORAGE_BACKEND=minio` and `STORAGE_BACKEND=s3` both use the S3-compatible
  adapter. `minio` forces path-style bucket lookup; `s3` uses SDK auto lookup
  unless `S3_FORCE_PATH_STYLE=true`.
- `S3_BUCKET_AUTO_CREATE=false` is the production default. Enable it only for
  local/dev smoke tests or tightly controlled single-server bootstrap.
- With auto-create disabled, API startup does not ping MinIO/S3; upload/download
  operations surface storage failures. With auto-create enabled, startup fails
  fast if the bucket check/create path cannot reach object storage.
- Use app-scoped object-store credentials, not MinIO root credentials, in
  production.
- The browser must still fetch through backend endpoints such as
  `GET /v1/files/{id}/content`; it must not receive bucket names, object keys,
  or direct MinIO/S3 URLs in the MVP.
- SDK `NoSuchKey` / missing-bucket style errors are mapped to
  `storage.ErrObjectNotFound` so the file API keeps returning
  `404 FILE_NOT_FOUND`.

Dependency note: the backend now targets Go 1.25. Keep
`github.com/minio/minio-go/v7` upgrades inside an explicit dependency review;
do not couple object-store SDK changes to unrelated feature work.

## Verification

Current Phase 6 tests cover:

- put/get/delete round trip
- metadata content type round trip
- unsafe key rejection
- size mismatch cleanup
- cancelled context cleanup
- MinIO/S3 config validation
- MinIO/S3 unsafe key rejection before network calls
- MinIO integration put/get/delete with `MM_CHAT_TEST_S3_*` environment
- API smoke with Postgres + MinIO upload/download/delete

## Phase 6.2 Usage

Phase 6.2 now uses this interface from the file service: upload writes object
bytes first, inserts Postgres metadata after SHA-256 is known, and removes the
object if metadata insertion fails. Downloads and deletes always resolve the
private object key from Postgres; browser responses never expose that key.

Phase 6.4 does not change the file HTTP contract. Switching from local storage
to MinIO/S3 is a deployment configuration change plus a metadata
`storage_backend` value change for newly uploaded files.
