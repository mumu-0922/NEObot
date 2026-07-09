# Phase 14 Production Hardening Plan

Phase 14 hardens the single-server Go backend before wider VPS deployment. Keep
frontend UI unchanged and make every operational change observable, reversible,
and documented in `docs/tracking/process.md`.

## 14.1 Request Logs and Readiness

Status: complete.

- Add request IDs and structured JSON request logs.
- Keep panic logs sanitized.
- Expose `/ready` dependency state for database, Redis, and storage.

## 14.2 Backup and Restore Drill

Status: complete.

- Verify Postgres logical dump and MinIO archive checksums.
- Restore only into temporary DB/bucket during drills.
- Cross-check restored Postgres file metadata with restored object bytes.
- Record cleanup evidence before marking progress complete.

## 14.3 Metrics Visibility

Status: complete.

Objective: expose low-risk Prometheus text metrics from the Go API without
adding a new monitoring stack yet.

Scope:

- Add public `GET /metrics` for same-host or reverse-proxy allowlisted scraping.
- Track API request count, status, bytes, and latency histogram with bounded
  route labels.
- Expose dependency readiness gauges for database, Redis, and storage. In the
  single-server MinIO deployment, the `storage` check represents MinIO/S3.
- Expose Postgres `database/sql` pool stats when the DB pool is configured.
- Document Prometheus scrape examples and basic PromQL.

Non-goals:

- No Grafana dashboard files in this slice.
- No OpenTelemetry tracing in this slice.
- No direct MinIO admin metrics scraping; object-storage readiness stays through
  the backend until a dedicated Prometheus/MinIO deployment plan is added.

Verification:

- Unit tests for `/metrics`, method handling, auth exemption, bounded route
  labels, dependency gauges, and DB stats.
- `go test ./... && go vet ./...` under `mm-chat/backend`.
- `curl http://127.0.0.1:8080/metrics` smoke when Docker backend is rebuilt.

Rollback:

- Revert the metrics handler/middleware commit. Existing `/health`, `/ready`,
  logs, and app routes are independent of `/metrics`.

## Remaining Phase 14 Work

- Reverse proxy and TLS production notes.
- Secret rotation notes.
- Optional Prometheus/Grafana compose profile after the `/metrics` endpoint is
  stable.
