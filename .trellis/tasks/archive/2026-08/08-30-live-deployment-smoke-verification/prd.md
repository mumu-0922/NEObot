# Live Deployment Smoke Verification

## Background

The standalone release gate has passed, but the already-running local deployment still needs a bounded end-to-end smoke through its public same-origin entrypoint. The smoke must exercise the configured live provider without changing provider configuration or contaminating existing user data.

## Goal

Verify that the deployed Neo Chat stack can complete and persist representative Chat and Agent flows, and—where existing resources permit—exercise RAG, Memory, Skill, and MCP integration paths. Every artifact created by the smoke must be isolated and removed afterward.

## Requirements

1. Preflight the active deployment before any provider request:
   - confirm the expected Compose services are running and healthy;
   - confirm the frontend same-origin entrypoint and backend health endpoint respond;
   - do not rebuild, restart, redeploy, or mutate runtime configuration.
2. Authenticate through a supported application path without exposing or requesting stored passwords, provider keys, session tokens, or other secrets in command output.
3. Use a unique smoke run identifier and create only dedicated temporary records.
4. Exercise a bounded live Chat request through the deployed frontend/API path and verify:
   - a terminal assistant response is produced;
   - the response is persisted and survives reload.
5. Exercise a bounded Agent request and verify:
   - at least one harmless tool call completes;
   - the run reaches a terminal state;
   - the interleaved process trace and final response survive reload.
6. Exercise RAG, Memory, Skill, and MCP only through resources already available to the authenticated account or through isolated temporary fixtures. Do not modify existing user documents, memories, selections, or installed resources.
7. Keep live-provider use small: one minimal successful request per required path, with no broad retries or media generation.
8. Delete all temporary conversations, files, memories, knowledge fixtures, and other smoke artifacts. Conversation audit/history rows may remain only under the product's normal soft-delete contract; verify they are inaccessible, no longer active, and have no live selection or runtime residue.
9. Record only non-secret results: pass/fail, durations, terminal states, and redacted or one-way identifiers where useful.
10. If a product defect is discovered, stop expanding the smoke, preserve evidence without secrets, and fix it only after identifying the root cause and defining focused verification.

## Acceptance Criteria

- All required deployment services pass preflight without restart or configuration mutation.
- A live-provider Chat response completes, persists, and reloads through the deployed path.
- A live-provider Agent run completes a harmless tool call, reaches a terminal state, and reloads with its process trace intact.
- Available RAG, Memory, Skill, and MCP paths are either verified with isolated data or explicitly reported as blocked with concrete evidence; no false pass is allowed.
- No existing conversation, workspace content, provider setting, memory, knowledge document, Skill, or MCP installation is changed.
- Every temporary artifact created by the smoke is removed or reduced to inaccessible product-authorized soft-delete history; active records, Sessions, selections, learn jobs, files, and other runtime residue are absent.

## Out of Scope

- Voice or image live-provider smoke, which has a separate authorization contract.
- Provider key, model, quota, or routing changes.
- Deployment rebuilds, restarts, migrations, or dependency updates.
- Backup/restore drill; that is the next independent operational task after this smoke closes.
- Performance, load, or concurrency testing.

## Execution Order

1. Deployment and authentication preflight.
2. Chat smoke and persistence reload.
3. Agent tool-loop smoke and persistence reload.
4. RAG, Memory, Skill, and MCP bounded checks where safe fixtures are available.
5. Cleanup verification.
6. Focused quality review, evidence record, commit, and task archive.

## Rollback and Safety

- No destructive command may target a broad directory, workspace root, or existing user record.
- Cleanup may target only artifacts whose identifiers were created and captured by this smoke run.
- If authentication cannot be established through a supported safe path, stop before provider calls and report the exact blocker.
- If cleanup fails, the task is not complete until the isolated artifacts are removed or precisely reported for manual recovery.
