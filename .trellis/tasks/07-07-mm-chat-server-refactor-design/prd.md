# mm-chat Server Refactor Design

Status: complete for the owner-approved single-server scope through G19. The
remaining hosted Voice/TTS integration and destructive former-root deletion
are explicitly deferred gates tracked outside this task's completion.

## Goal

Rebuild Neo Chat as a server-backed deployment whose final, self-contained
project root is `mm-chat/`. During migration the existing root application is a
temporary compatibility source, but after verified feature/data/build/deploy
cutover it must be removable without breaking `mm-chat/`.

## What I already know

- User wants all future refactor work placed under `mm-chat/`.
- Existing Neo Chat is a Next.js/React TypeScript app with local-first storage.
- Current file bytes are browser-side via OPFS; app metadata uses IndexedDB/localforage.
- No existing S3/MinIO integration was found.
- Target runtime architecture: Next.js frontend, Go backend, Postgres, Redis,
  MinIO on one server, plus the private Python RAG service.
- The complete Next.js frontend now lives under `mm-chat/frontend/`; clean-copy
  install, test, build, runtime, Compose, backup/restore, and visual gates pass
  without the former root application.
- The owner requires `mm-chat/` to remain the only independently runnable
  complete project directory. Former-root deletion is intentionally separate
  and cannot occur without its exact one-shot authorization.
- The capability inventory is resolved through Go-backed migration or explicit
  owner decisions: Teams were intentionally removed for a single-user product,
  hosted Voice/TTS remains deferred behind the existing Go seam, and destructive
  root deletion remains pending. No capability was silently dropped.
- The existing frontend interface is the visual and interaction baseline. The
  backend is the current Go refactor, with Python kept behind Go for private RAG
  workloads; future features must join the same frontend theme and component
  language instead of introducing a separate UI shell.
- The owner selected a server-only final runtime. Browser-local storage is not
  production authority; old browser data remains recoverable through the
  explicit import flow.

## Requirements

- Create `mm-chat/` at repo root.
- Do not modify existing source files outside the new folder for this design task.
- Add a detailed refactor design document.
- Add a checklist/progress document where completed items can be checked off.
- Add a process log document to record work completed and decisions made.
- Plan must support incremental migration and rollback.
- Single-server deployment must be the initial target.
- Make `mm-chat/frontend/` a complete Next.js application with its own manifest,
  configuration, assets, tests, build, and runtime entrypoint.
- Eliminate build, test, runtime, deployment, fixture, and documentation
  dependencies from `mm-chat/` back to the original root application.
- Migrate or explicitly retire every legacy root capability before deletion;
  no silent feature loss is allowed.
- Preserve the existing frontend information architecture, visual theme, and
  established interaction patterns while relocating it into
  `mm-chat/frontend/`; this migration is not a redesign.
- Reimplement supported legacy server capabilities behind the Go backend
  contract and record every owner-approved retirement or deferral;
  browser-visible code must not retain production dependence on root Next.js
  `/api/*` handlers.
- Remove the production `local|server` dual path after parity and cutover. The
  final frontend talks to Go APIs only, while the browser-data importer remains
  available for deliberate one-time migration.
- Require future feature UI to reuse the migrated design tokens, layout,
  components, accessibility behavior, and responsive conventions.
- Add a final clean-copy gate that builds, tests, starts, backs up/restores, and
  exercises `mm-chat/` without the original root tree present.
- Preserve an auditable backup/tag and rollback procedure before destructive
  deletion of the original project.

## Acceptance Criteria

- [x] `mm-chat/README.md` explains the purpose and document map.
- [x] `mm-chat/docs/architecture/server-refactor-design.md` contains the detailed architecture and phased refactor plan.
- [x] `mm-chat/docs/tracking/progress.md` contains checkboxes for every major phase.
- [x] `mm-chat/docs/tracking/process.md` records this initial design-generation step.
- [x] No existing application source file is modified by this task.
- [x] `mm-chat/frontend/` is independently installable, testable, buildable, and
      runnable.
- [x] `mm-chat/` has no required path/import/workspace/build/runtime dependency
      on the original root application.
- [x] Every original frontend and `/api/*` capability is mapped to migrated,
      intentionally retired, or explicitly deferred status with owner approval.
- [x] The frozen legacy feature inventory is resolved through Go-backed
      contracts or explicit owner-approved retirement/deferral; no existing
      user-visible capability is silently removed.
- [x] Visual regression and interaction smoke tests prove that the migrated
      frontend preserves the existing interface across agreed desktop/mobile
      viewports.
- [x] A clean-copy deployment using only `mm-chat/` passes the frozen parity,
      security, backup/restore, and rollback gates.
- [x] The original-project deletion guard requires a separate exact delete plan
      and one-shot owner confirmation; execution remains pending under G10.4b.

## Definition of Done

- Docs are concise but actionable.
- Each phase has objective, actions, outputs, verification, and rollback notes.
- Progress checklist starts with completed document-scaffolding items checked.
- Process log records date, action, evidence, and next step.

## Technical Approach

Use an incremental strangler migration that converges on a self-contained
`mm-chat/` tree rather than preserving two permanent project roots:

- Move the existing Next.js application into `mm-chat/frontend/` as the UI
  baseline before making visual changes.
- Replace legacy `/api/*` dependencies capability by capability with typed Go
  API adapters while keeping the visible interface stable.
- Centralize the existing theme tokens and reusable components so later
  features extend the same design language.

- `README.md` — entrypoint and rules for future refactor work.
- `docs/architecture/server-refactor-design.md` — architecture, migration phases, API/data/storage/deploy design.
- `docs/tracking/progress.md` — living checklist; mark items when completed.
- `docs/tracking/process.md` — chronological execution log.

## Decision (ADR-lite)

**Context**: The existing repo is local-first and has many uncommitted changes. Directly modifying the current app during planning would increase rollback risk.

**Decision**: Isolate all new refactor artifacts under `mm-chat/` and use
strangler migration: preserve the current root frontend only as a temporary
compatibility source, introduce a Go backend behind an API boundary, migrate
capabilities and data gradually, then consolidate the complete frontend under
`mm-chat/frontend/` and retire the original root project after independent
clean-copy verification.

**Consequences**: The plan keeps rollback available during migration, but the
project is not complete while either runtime still requires root `src/`,
`public/`, root manifests, or root `/api/*` handlers. Final deletion is a
separate destructive gate, never an implicit cleanup step.

**Product scope decision**: Every frozen capability requires Go-backed parity or
an explicit owner-approved retirement/deferral. Teams are intentionally retired
for the single-user product and hosted Voice/TTS is deferred; neither is silent
feature loss. The current frontend UI/theme remains the product baseline.

**Runtime decision**: The final application is server-only. Browser-local
authority has been retired from production; the explicit browser-data importer
remains available for deliberate migration.

## Initial Design-Slice Out of Scope (historical)

- Implementing Go backend code in this task.
- Changing existing Next.js frontend files in this task.
- Adding Docker services in root compose files in this task.
- Migrating existing browser data automatically in this task.
- Deleting the original root project before standalone parity and clean-copy
  verification pass.
- Redesigning or replacing the existing frontend visual language during the
  consolidation migration.

## Technical Notes

- Relevant existing docs: `CONTRIBUTING.md`, `docs/privacy-and-local-data.md`, `src/store/README.md`.
- Storage evidence: OPFS and localforage are used; no S3/MinIO matches found in project code/config.
- Single-server first deployment: Docker Compose, local volumes, MinIO private behind backend.
- Remaining migration scope must be derived from the checked process/progress
  records and then verified against live routes, imports, manifests, Compose,
  and runtime behavior; documentation alone cannot mark a capability migrated.
