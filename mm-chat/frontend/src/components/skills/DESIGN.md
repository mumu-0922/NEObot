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
`services/api/client/server/skillStoreApi.ts`. Install binds the candidate ID to
the displayed package fingerprint. Uninstall uses the current installation
revision. Stale revisions reload server authority instead of applying an
optimistic local result.

Package descriptions, declared tools, fingerprints, and status values render as
React text. The surface never renders package HTML and never exposes Runner or
control-plane credentials.

## Interaction

Desktop renders list and detail together. Mobile drills into the selected
package and restores focus to the originating list item on Back. Loading,
failures, installs, and removals have accessible live feedback.
