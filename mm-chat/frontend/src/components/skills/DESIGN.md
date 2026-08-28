# Skill Store Design

## Responsibility

The Skill Store owns package discovery and the current user's installed Skill
library. PostgreSQL and `/v1/skills/*` remain authoritative; the component owns
only transient list, selection, loading, error, and announcement state.
Its list selection is only a detail-panel cursor: durable per-Conversation
Skill enablement belongs to `ConversationResourcePickers` and is never changed
by install or uninstall UI implicitly.

## Trust boundary

Every server response is validated by strict Zod schemas in
`services/api/client/server/skillStoreApi.ts`. Catalog detail is fixed to the
OpenAI curated repository/path shape. Install binds the Skill name, resolved
commit, and displayed package fingerprint. Uninstall uses the current
installation revision. Stale revisions reload server authority instead of
applying an optimistic local result.

Package descriptions, declared tools, source metadata, and status values render
as React text. Raw fingerprints, admission IDs, SBOM data, package HTML, Runner
details, and control-plane credentials are not normal product language.

## Interaction

The top-level surface follows the Tools information architecture with mutually
exclusive `Installed` and `Skill Store` tabs. Installed packages render only in
the library tab. Curated Skills render in the Store list/detail view;
desktop keeps that list and detail together, while mobile drills into the
selected package and restores focus to the originating list item on Back.
Installing refreshes both authorities, clears the selected candidate URL, and
returns to Installed. Loading and failure state is isolated per tab, while
installs and removals have accessible live feedback.

Store and Installed cards share `SkillIcon.tsx`. Safe GitHub avatars and short
emoji use the existing bounded path; installations without artwork render the
same deterministic label fallback as curated Store entries and never trigger a
Marketplace lookup from the Library view.
