# Restore installed Skill icons

## Goal

Make every installed Skill card show the same bounded icon/fallback treatment
as Skill Store cards, so installation does not visually strip the Skill
identity.

## What I already know

- `SkillMarketplacePrimitives.tsx` already owns the safe `SkillIcon` renderer.
- `SkillIcon` supports allowlisted remote images, short emoji, and a
  deterministic first-letter fallback.
- `InstalledSkills` currently renders name/version/description but never calls
  that renderer.
- Current installation DTOs do not expose an icon. The deterministic label
  fallback is therefore the authoritative behavior for existing installed
  entries such as `pdf` and `grill-me`.

## Requirements

- Reuse one shared `SkillIcon` component in Store cards and Installed cards.
- Render the icon before the installed Skill's text without weakening the
  existing remote-image URL allowlist.
- Preserve uninstall, loading, keyboard, responsive, dark-mode, and text
  behavior.
- Existing installations require no reinstall, migration, or network lookup.

## Acceptance Criteria

- [x] Every installed card has a visible 44px icon/fallback.
- [x] `pdf` deterministically renders `P`; `grill-me` renders `G` when no icon
  is supplied.
- [x] Store cards continue using the same renderer and security allowlist.
- [x] Focused frontend format/lint/typecheck/tests pass.
- [x] Frontend is rebuilt and healthy.

## Definition of Done

- Shared renderer exported and used by Installed Skills.
- Focused regression coverage prevents the icon call from disappearing.
- Deployment is rebuilt, work committed, task archived, and journal recorded.

## Technical Approach

Export `SkillIcon` from `SkillMarketplacePrimitives.tsx`, import it into
`SkillStorePrimitives.tsx`, and place it in the installed-card flex layout.
Keep `icon` optional so the existing installation DTO remains unchanged and
the established label fallback is used.

## Decision (ADR-lite)

**Context**: Existing installed records do not persist Marketplace display
icons, while every Skill still needs a stable visual marker.

**Decision**: Reuse the Store's deterministic fallback now instead of adding a
database field or making the installed Library depend on live Marketplace
availability.

**Consequences**: Existing installs gain icons immediately and remain offline
capable. A future source-icon persistence change can pass an optional icon into
the same renderer without another UI redesign.

## Out of Scope

- Database or installation DTO changes.
- Fetching LobeHub during Library reads.
- Backfilling third-party source artwork.
- Redesigning the installed card beyond adding the missing icon.

## Technical Notes

- Relevant spec: `.trellis/spec/frontend/skill-store.md` if present, otherwise
  component-local `components/skills/DESIGN.md` and Backend Skill supply-chain
  authority.
- Likely files: `SkillMarketplacePrimitives.tsx`,
  `SkillStorePrimitives.tsx`, and focused Skill Store tests.
