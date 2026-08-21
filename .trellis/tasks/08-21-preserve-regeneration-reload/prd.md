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

- [ ] Server reload of `User -> old Assistant -> follow-up -> answer -> regenerated Assistant` projects `User -> regenerated Assistant -> same follow-up -> same answer`.
- [ ] Every node remains reachable and the moved direct child points at the regenerated Assistant node.
- [ ] Existing live regeneration tests remain green.
- [ ] Focused tests, format/lint, typecheck, production build, rollout, and live health pass.

## Technical Approach

Teach `appendServerMessageToTree` to detect an existing active Assistant sibling under the resolved User parent and route the newer Assistant through `createModelResponseBranch`. Because `normalizeServerMessageTree` already reduces chronological server messages through this helper, reload and live insertion converge without a schema or Backend change.

## Out of Scope

- Persisting UI version-selection events in Backend storage.
- API or schema changes.
- Unrelated full test suites.
