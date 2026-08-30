# Deployment Result

## Release

- Source fix: `ae221181`
- Deployed version: `skill-cleanup-ae221181`
- Candidate image: `mm-chat/backend:skill-cleanup-ae221181`
- Recreated service: Backend only
- Rollback image retained:
  `mm-chat/backend:memory-orphan-activity-20260828T234500Z`

## Recovery rehearsal

- Matched backup ID: `predeploy-ae221181-20260830T091029Z`
- PostgreSQL dump and MinIO archive both have mode `0600` and verified
  SHA-256 sidecars.
- PostgreSQL restored into a unique temporary database.
- Restored and live migration manifests matched across all 109 rows.
- Thirteen representative Chat, File, Knowledge, Memory, Skill, and MCP table
  counts matched.
- No unvalidated foreign key or Knowledge Document Version/File metadata
  mismatch was found.
- MinIO restored 135 objects into a temporary bucket and verified 5 Knowledge,
  2 MCP-result, and 12 Skill-supply object samples.
- The temporary database, bucket, extraction directory, and sample files were
  removed after verification.

## Deployment verification

- Candidate image declares user `mmchat:mmchat` and API command
  `/usr/local/bin/mm-chat-api`.
- Compose rendered the candidate with only the expected release-identity and
  image-owned `PATH` differences from the old Backend container.
- Direct Backend readiness, same-origin readiness, and Frontend HTTP checks
  passed.
- All eight application services are running with no unhealthy service.
- Every unrelated service retained its exact pre-deploy container ID.
- Live migration manifest and bounded row-count baseline were unchanged.
- No Backend startup `ERROR`, `FATAL`, or panic evidence was found.
- Agent Host remained healthy.

## Target regression

- A short-lived authenticated session created a marked temporary Conversation.
- One existing installed Skill was bound at selection revision 1.
- Public same-origin Conversation deletion returned `204`.
- The deleted Conversation disappeared from the list while retaining the
  expected soft-delete database state.
- Both the Skill selection and its installation-item rows became zero.
- Installed Skill Library count remained 2 before and after deletion.
- No Provider request was made.
- The exact temporary Conversation and Session were removed; bounded live row
  counts returned to their pre-test values.

## Operational finding

The checked-in PostgreSQL restore example still hard-codes migration authority
through migration 10, while the live schema is at 109. Restore acceptance must
compare the live and restored `(version, name, checksum)` manifests dynamically
instead of maintaining another migration list in the runbook.
