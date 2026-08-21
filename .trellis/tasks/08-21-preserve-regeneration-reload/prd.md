# Preserve Regenerated Continuations After Reload

## Goal

Make the regeneration continuation projection durable across Server conversation reloads by reconstructing sibling Assistant answers as one answer slot from chronological server messages.

## Requirements

- During Server message-tree normalization, adding a second Assistant child under the same User parent must transfer the active sibling's descendants to the newer Assistant sibling.
- Normal first Assistant responses and User edit branches retain existing behavior.
- Live regeneration and terminal-only regeneration use the same tree invariant.
- Add a reload regression test proving a persisted old-answer continuation remains visible below the newest regenerated answer.
- Run focused Frontend checks, build and deploy only Frontend, commit automatically, and do not Push.

## Acceptance Criteria

- [x] Server reload of `User -> old Assistant -> follow-up -> answer -> regenerated Assistant` projects `User -> regenerated Assistant -> same follow-up -> same answer`.
- [x] Every node remains reachable and the moved direct child points at the regenerated Assistant node.
- [x] Existing live regeneration tests remain green.
- [x] Focused tests, format/lint, typecheck, production build, rollout, and live health pass.

## Technical Approach

Teach `appendServerMessageToTree` to detect an existing active Assistant sibling under the resolved User parent and route the newer Assistant through `createModelResponseBranch`. Because `normalizeServerMessageTree` already reduces chronological server messages through this helper, reload and live insertion converge without a schema or Backend change.

## Out of Scope

- Persisting UI version-selection events in Backend storage.
- API or schema changes.
- Unrelated full test suites.

## Rollout Evidence

- Work commit: `6b687638`.
- Frontend image: `mm-chat/frontend:regen-reload-6b687638-20260821T094930Z`
  (`sha256:338595990f914c5a55cf97167ee746f8ce6b273d2b232e9d3fc3f73b612a7b75`).
- Rollback snapshot:
  `mm-chat/backup/deployments/20260821T094930Z-regen-reload-6b687638/`.
- Focused verification passed: 77 tree/Local-store/Server-store tests,
  changed-file formatting and ESLint, TypeScript typecheck, and host/Docker
  production builds. No unrelated full suite was run.
- Live root and `/api/health` returned HTTP `200`; Frontend and unchanged
  Backend are healthy.
- Rollback restores the snapshot environment and recreates only `frontend`;
  no schema or data rollback is required.
