# Make recall filtering Provider configurable

## Goal

Allow the owner to move the Memory recall-filtering judge away from the
quota-exhausted `SERVER_DEFAULT` Provider without editing runtime data by hand.
The settings UI must persist the selected Provider authority in Go/Postgres and
the answer-time Memory candidate judge must use that exact authority.

## What I already know

- The user selected the program-change option because the current fixed
  `Sub / SERVER_DEFAULT` Provider has no quota.
- The existing UI renders recall filtering as a read-only
  `GPT-5.6 Luna · system fixed` value.
- The six ordinary task-model selections already use server-owned
  `providerId:modelId` references and immediate PATCH persistence.
- The production Memory Judge is calibrated around the fixed
  `SERVER_DEFAULT/openai_compatible/gpt-5.6-luna` tuple and a 3000 ms policy.
- Runtime authority additionally pins the historical `sub.mumubuku.top` Base
  URL by SHA-256, so merely editing the `SERVER_DEFAULT` Provider would fail
  closed as provenance drift.
- The target `New Provider` is enabled, backend-stored, exposes
  `gpt-5.6-luna`, and currently points at `cpa.mumubuku.top`.

## Requirements

- Make the Provider selectable while keeping
  `gpt-5.6-luna` fixed, because the judge prompt, decoder, thresholds, and
  regression evidence are model-specific.
- Existing installations without a new selection must preserve the current
  `SERVER_DEFAULT:gpt-5.6-luna` behavior.
- Invalid, deleted, disabled, non-OpenAI-family, or Luna-missing Providers
  must fail closed and surface a bounded health/configuration error.
- Replace the read-only recall-filtering row with a server-persisted selection.
- Resolve and use the selected Provider at answer time; never silently fall
  back to `SERVER_DEFAULT` after an explicit selection.
- Keep secrets server-side and validate the selected Provider against the
  backend-stored enabled model catalog.
- Preserve backward compatibility for owners with no new setting.
- Update Memory health to report the effective Provider/model authority
  without exposing credentials.
- Add migration, repository, service, handler, runtime wiring, frontend, and
  regression coverage proportional to this cross-layer persistence change.

## Acceptance Criteria

- [x] The settings page can select `New Provider` for recall filtering and
      persists the choice across refresh and backend restart.
- [x] A saved selection resolves to the exact backend-stored Provider at
      answer time and invokes `gpt-5.6-luna` through it.
- [x] Existing rows with no selection retain the legacy server-default path.
- [x] A stale/disabled/incompatible selection fails closed with no unjudged
      Memory candidates injected.
- [x] The UI rolls back an optimistic selection if server validation fails.
- [x] Provider secrets never enter the browser, logs, task settings, or Git.

## Definition of Done

- Frontend format, lint, typecheck, focused Vitest, and build pass.
- Backend vet, focused/full Go tests, migration replay/schema checks pass.
- Relevant Memory/task-model contracts and operational notes are updated.
- Live proof switches recall filtering to the new Provider, reloads it, and
  verifies the old `Sub` authority is not used for a controlled recall turn.
- Rollback preserves existing settings and runtime user data.

## Technical Approach

Extend the server-owned task-model settings contract with a dedicated recall
filtering `providerId:gpt-5.6-luna` reference. The full reference reuses the
existing bounded task-model persistence contract, while the UI presents only
enabled OpenAI or OpenAI-compatible Providers that expose the fixed Luna model.
The backend resolves the selected Provider and continues to construct the
existing fixed-model Memory Judge adapter. Runtime resolution preserves the
historical server-default path only when the new setting is absent; an explicit
stale selection fails closed.

## Decision (ADR-lite)

**Context:** The current Memory Judge pins both `SERVER_DEFAULT` and the
historical Sub endpoint. Sub has exhausted its quota, but making the model
arbitrary would invalidate the model-specific prompt, decoder, thresholds, and
regression evidence.

**Decision:** Make only the Provider selectable and keep
`gpt-5.6-luna` fixed. Persist the Provider authority on the server and resolve
it at answer time.

**Consequences:** The owner can route the calibrated judge through another
enabled OpenAI or OpenAI-compatible Provider that exposes Luna. Endpoint
provenance is no longer globally pinned to Sub, so the effective Provider
identity must be reported and stale selections must fail closed. Supporting
another judge model still requires separate calibration work.

## Verification Evidence

- Migration `105` applied live and schema head verified after a paired,
  checksummed PostgreSQL/MinIO backup.
- `PJRSVY:gpt-5.6-luna` passed the stored connection fingerprint and catalog
  guards, was persisted, and remained selected after Backend recreation.
- Backend full tests/vet and Frontend format/lint/typecheck/focused tests/build
  passed. The configured-provider integration test sends exact Luna to an
  `OpenAI` Chat Completions fixture; stale explicit authority never calls the
  legacy resolver.
- No paid Provider request was made during deployment because the owner will
  supply the controlled question/answer acceptance test.

## Out of Scope

- Recalibrating judge prompts, thresholds, BGE retrieval, or Memory policies.
- Making the recall-filtering model itself selectable.
- Changing the independent SiliconFlow RAG retrieval Provider.
- Migrating ordinary chat conversations or the six existing task selections.

## Technical Notes

- UI: `mm-chat/frontend/src/components/settings/DefaultModelSettings.tsx`
- Task model authority: `mm-chat/backend/internal/runtimeconfig/task_models.go`
- Judge wiring and historical endpoint pin:
  `mm-chat/backend/internal/httpserver/server.go`
- Fixed model/policy constants: `mm-chat/backend/internal/usermemory/types.go`
- Existing persistence migration:
  `mm-chat/backend/migrations/036_task_model_settings.up.sql`
- Contracts:
  `mm-chat/docs/tracking/g15-task-model-settings-plan.md` and
  `mm-chat/backend/internal/memorycapture/DESIGN.md`
