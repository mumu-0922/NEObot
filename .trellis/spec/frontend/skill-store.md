# Skill Store

## Scope

The top-level Skill Store is the only user-facing management surface for Agent
Skill packages. It discovers admitted packages and installs/uninstalls the
current user's library through `/v1/skills/*`.

Runs, Schedules, Learning, Shadow, Canary, delegation, Runner, and OCI controls
are retired product concepts and must not reappear in this surface.

## URL and navigation

- Canonical panel: `panel=skill-store`.
- Optional selected package: `skillId=<validated candidate id>`.
- The page exposes `Installed | Skill Store` tabs. Plain navigation opens
  Installed; an initial search or valid `skillId` opens Skill Store.
- Installing refreshes both server-authoritative lists, clears `skillId`, and
  returns to Installed. Installed and Store loading/error state remain isolated.
- Old `panel=agent-center&agentTab=skills` URLs migrate to the Skill Store.
- Other legacy `agentTab`/`agentId` state is discarded.
- Desktop keeps list/detail visible; mobile drills into detail and restores
  focus to the originating package on Back.
- A slash/resource search may target an admitted candidate beyond the first
  Store page. If `skillId` is not in the loaded list, fetch that exact candidate
  through `getPackageSkill` before rendering detail; never treat first-page
  absence as non-existence.

## API and authority

- The frontend client exposes `skillStore`, not `agentCenter`.
- The server adapter calls only `/v1/skills/store`, `/v1/skills/library`, and
  their package install/detail endpoints.
- Strict Zod schemas validate every response before rendering.
- Install binds candidate ID to the displayed package fingerprint.
- Uninstall sends the current installation revision; stale conflicts reload
  PostgreSQL authority.
- Local browser mode reports the server-owned feature as unsupported.
- The Store tab consumes the existing admitted catalog. Choosing or integrating
  a public Marketplace is a separate backend-provider decision; the frontend
  must not scrape or bind itself to an external marketplace protocol.

## Security and UX

- Render descriptions, status, tools, and fingerprints as React text only.
- Never render package HTML or expose package file content in the Store.
- Keep accessible names, live announcements, retry state, keyboard focus
  restoration, dark mode, and responsive list/detail behavior.

## Required tests

- Skill Store tab composition, Installed/Store separation, post-install return,
  and absence of retired control terms.
- Server API route, strict DTO, install, and revision-bound uninstall tests.
- URL round-trip plus legacy Agent Center Skills migration tests.
- Sidebar navigation, format, lint, typecheck, Vitest, and production build.
