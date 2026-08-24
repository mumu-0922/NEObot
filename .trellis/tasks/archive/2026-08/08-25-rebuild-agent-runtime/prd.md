# Rebuild and verify completion-driven Agent runtime

## Goal

Rebuild and replace the running single-server Backend with the committed
completion-driven Agent runtime, then verify that the deployed service is
healthy and that the old whole-run timeout regression remains closed.

## Requirements

- Use the existing `mm-chat/.env.single-server` and
  `mm-chat/compose.single-server.yml` deployment authority.
- Rebuild only the Backend image and recreate only the Backend service unless
  a dependency health failure requires further action.
- Preserve PostgreSQL, Redis, MinIO, Frontend, RAG worker, MCP Runner, and all
  runtime data.
- Verify the Backend container and public health/readiness endpoints after the
  replacement.
- Run the focused completion-driven Agent regression tests and the local Agent
  runtime verification gate.
- Inspect sanitized Backend logs for startup, migration, panic, and fatal
  failures.

## Acceptance Criteria

- [x] The Backend image is rebuilt from current `main` at commit `60e67df7` or
      later and the Backend container is recreated from that image.
- [x] `docker compose ... ps` reports Backend healthy without degrading the
      existing dependency services.
- [x] `GET /health` succeeds and `GET /ready` reports ready.
- [x] Focused Go tests covering completion past the former deadline and
      no-progress blocking pass.
- [x] `verify-agent-local-runtime.sh` passes.
- [x] Recent Backend logs contain no panic or fatal startup failure.

## Definition of Done

- Deployment replacement and verification evidence are recorded.
- No application source change is introduced unless rebuilding exposes a real
  defect; any such defect must be separately diagnosed before modification.
- The task is archived and the operational session is recorded.

## Technical Approach

Render and validate the existing Compose authority, build the configured
Backend image, recreate only `backend`, wait for health, then run HTTP probes,
focused Go regressions, the project Agent verification script, and a bounded
sanitized log inspection.

## Decision (ADR-lite)

**Context**: The committed change is Backend-only while the running Backend
still uses the previous Agent runtime image.

**Decision**: Perform a Backend-only rolling replacement through the existing
single-server Compose file rather than rebuilding or restarting the full stack.

**Consequences**: Downtime is limited to the Backend restart and runtime data
services remain untouched. Existing in-flight Agent runs on the old Backend
will be interrupted by the process replacement.

## Out of Scope

- Rebuilding Frontend, RAG, MCP Runner, Memory worker, or data services.
- A paid five-minute live Provider run; deterministic regression coverage is
  used for the former timeout boundary.
- Changing Agent runtime behavior or deployment configuration.

## Technical Notes

- Product root: `mm-chat/`.
- Deployment runbook: `mm-chat/docs/deployment/agent-runtime.md`.
- Runtime spec: `.trellis/spec/backend/agent-runtime.md`.
- Current Backend image before replacement:
  `mm-chat/backend:agent-terminal-contract-20260824T062515Z`.
- The user explicitly approved rebuild and testing in this conversation.
