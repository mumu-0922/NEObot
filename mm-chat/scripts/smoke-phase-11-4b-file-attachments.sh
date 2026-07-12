#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-${BASE_URL:-http://127.0.0.1:8080}}"
RUN="${RUN:-phase-11-4b-file-smoke-$(date +%s)-$$}"
ARTIFACT_DIR="${ARTIFACT_DIR:-/tmp/mm-chat-smoke/$RUN}"
FILENAME="${RUN}.txt"
PAYLOAD="$ARTIFACT_DIR/payload.txt"
DOWNLOADED="$ARTIFACT_DIR/downloaded.txt"

for command in curl jq sha256sum cmp; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'required command not found: %s\n' "$command" >&2
    exit 127
  fi
done

mkdir -p "$ARTIFACT_DIR"
printf 'neo-chat %s\n' "$RUN" > "$PAYLOAD"
LOCAL_SHA="$(sha256sum "$PAYLOAD" | awk '{print $1}')"

curl -fsS "$BASE/health" >/dev/null
curl -fsS "$BASE/ready" >/dev/null
curl -fsS "$BASE/v1/version" -o "$ARTIFACT_DIR/version.json"

curl -fsS -X POST "$BASE/v1/files" \
  -H "Accept: application/json" \
  -F "file=@${PAYLOAD};filename=${FILENAME};type=text/plain" \
  -F "purpose=chat" \
  -F "clientFileId=$RUN" \
  -o "$ARTIFACT_DIR/upload.json"

FILE_ID="$(jq -r '.id' "$ARTIFACT_DIR/upload.json")"

jq -e --arg id "$FILE_ID" --arg sha "$LOCAL_SHA" --arg name "$FILENAME" '
  (.id == $id)
  and (.fileName == $name)
  and (.mimeType == "text/plain")
  and (.sha256 == $sha)
  and (.purpose == "chat")
  and (.downloadUrl == ("/v1/files/" + $id + "/content"))
  and ((has("objectKey") or has("bucket") or has("storageBackend") or has("localPath") or has("presignedUrl")) | not)
' "$ARTIFACT_DIR/upload.json" >/dev/null

curl -fsS "$BASE/v1/files/$FILE_ID" \
  -H "Accept: application/json" \
  -o "$ARTIFACT_DIR/metadata.json"

jq -e --arg id "$FILE_ID" --arg sha "$LOCAL_SHA" '
  (.id == $id)
  and (.sha256 == $sha)
  and (.downloadUrl == ("/v1/files/" + $id + "/content"))
  and ((has("objectKey") or has("bucket") or has("storageBackend") or has("localPath") or has("presignedUrl")) | not)
' "$ARTIFACT_DIR/metadata.json" >/dev/null

curl -fsS "$BASE/v1/files/$FILE_ID/content?disposition=attachment" \
  -H "Accept: application/octet-stream, */*" \
  -o "$DOWNLOADED"

cmp -s "$PAYLOAD" "$DOWNLOADED"

curl -fsS -X POST "$BASE/v1/chat/conversations" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  --data "$(jq -n --arg run "$RUN" \
    '{title:("Phase 11.4B file smoke " + $run),
      idempotencyKey:($run + ":conversation"),
      config:{smokeRun:$run}}')" \
  -o "$ARTIFACT_DIR/conversation.json"

CONVERSATION_ID="$(jq -r '.id' "$ARTIFACT_DIR/conversation.json")"

curl -fsS -X POST "$BASE/v1/chat/conversations/$CONVERSATION_ID/messages" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  --data "$(jq -n --arg run "$RUN" --arg fileId "$FILE_ID" \
    '{content:"Phase 11.4B attachment smoke",
      idempotencyKey:($run + ":message"),
      metadata:{smokeRun:$run},
      attachments:[{source:"server", fileId:$fileId, purpose:"input"}]}')" \
  -o "$ARTIFACT_DIR/message.json"

MESSAGE_ID="$(jq -r '.id' "$ARTIFACT_DIR/message.json")"

jq -e --arg mid "$MESSAGE_ID" --arg fileId "$FILE_ID" --arg sha "$LOCAL_SHA" --arg name "$FILENAME" '
  (.id == $mid)
  and (.role == "user")
  and (.status == "completed")
  and (.attachments | length == 1)
  and (.attachments[0].source == "server")
  and (.attachments[0].fileId == $fileId)
  and (.attachments[0].fileName == $name)
  and (.attachments[0].mimeType == "text/plain")
  and (.attachments[0].sha256 == $sha)
  and (.attachments[0].purpose == "input")
  and ((.attachments[0] | (has("objectKey") or has("bucket") or has("storageBackend") or has("localPath") or has("downloadUrl") or has("presignedUrl"))) | not)
' "$ARTIFACT_DIR/message.json" >/dev/null

curl -fsS "$BASE/v1/chat/conversations/$CONVERSATION_ID/messages" \
  -H "Accept: application/json" \
  -o "$ARTIFACT_DIR/messages.json"

jq -e --arg mid "$MESSAGE_ID" --arg fileId "$FILE_ID" --arg sha "$LOCAL_SHA" --arg name "$FILENAME" '
  ([.items[] | select(.id == $mid)] | length) == 1
  and (
    ([.items[] | select(.id == $mid)][0].attachments[0]) as $a
    | ($a.source == "server")
      and ($a.fileId == $fileId)
      and ($a.fileName == $name)
      and ($a.mimeType == "text/plain")
      and ($a.sha256 == $sha)
      and ($a.purpose == "input")
      and (($a | (has("objectKey") or has("bucket") or has("storageBackend") or has("localPath") or has("downloadUrl") or has("presignedUrl"))) | not)
  )
' "$ARTIFACT_DIR/messages.json" >/dev/null

printf 'run=%s\nfileId=%s\nconversationId=%s\nmessageId=%s\nartifacts=%s\n' \
  "$RUN" "$FILE_ID" "$CONVERSATION_ID" "$MESSAGE_ID" "$ARTIFACT_DIR"
