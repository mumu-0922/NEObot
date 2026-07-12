#!/bin/sh
set -eu

: "${S3_BUCKET:?S3_BUCKET is required}"
: "${MINIO_ROOT_USER:?MINIO_ROOT_USER is required}"
: "${MINIO_ROOT_PASSWORD:?MINIO_ROOT_PASSWORD is required}"

alias_name="rootdrill"
endpoint="${S3_ENDPOINT:-http://minio:9000}"
drill_bucket="${S3_BUCKET}-restore-drill-$(date +%Y%m%d%H%M%S)-$$"
bucket_created=false

cleanup() {
  if [ "$bucket_created" = true ]; then
    mc rb --force "${alias_name}/${drill_bucket}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT HUP INT TERM

mc alias set "$alias_name" "$endpoint" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc mb "${alias_name}/${drill_bucket}" >/dev/null
bucket_created=true
mc mirror --overwrite --remove "/restore-source/${S3_BUCKET}" "${alias_name}/${drill_bucket}" >/dev/null

mc find "${alias_name}/${drill_bucket}" > /tmp/mm-chat-restore-objects.txt
object_count=0
while IFS= read -r object_path; do
  if [ -n "$object_path" ]; then
    object_count=$((object_count + 1))
  fi
done < /tmp/mm-chat-restore-objects.txt
echo "restored_object_count=${object_count}"

sample_count=0
while IFS= read -r object_key; do
  if [ -n "$object_key" ]; then
    mc stat "${alias_name}/${drill_bucket}/${object_key}" >/dev/null
    sample_count=$((sample_count + 1))
  fi
done < /knowledge-object-sample.txt
echo "knowledge_document_version_objects_checked=${sample_count}"

mc rb --force "${alias_name}/${drill_bucket}" >/dev/null
bucket_created=false
echo "cleanup=drill_bucket_removed"
