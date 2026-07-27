# Patch frontend dependency vulnerabilities

## Goal

Remove the currently audited high-severity frontend production dependency
findings without changing MM Chat product behavior, provider contracts, API
routes, or deployment topology.

## What I already know

- `mm-chat/frontend` installs `next@16.2.9` from the frozen lockfile.
- An official-registry production audit reports 15 findings, including eight
  high-severity findings.
- Four high findings affect Next.js versions `<16.2.11`; the current stable
  compatible patch is `16.2.12`.
- The application uses Next middleware for API access control and a rewrite for
  the private Go backend, so the audited Middleware/Proxy and rewrite findings
  are relevant patch targets.
- High transitive findings also affect `sharp`, `postcss`, and
  `brace-expansion`; the workspace already owns targeted dependency overrides.
- The clean-copy standalone gate passed before this task with 936 frontend
  tests, all Go tests, and 1,906 RAG tests passing with seven integration skips.

## Requirements

- Update `next` and `eslint-config-next` together to the current patched
  `16.2.x` release.
- Update only the direct build/runtime packages needed to obtain compatible
  patched transitive dependency graphs.
- Extend the existing `pnpm-workspace.yaml` security overrides for vulnerable
  `sharp`, `postcss`, `brace-expansion`, and `js-yaml` ranges when parent
  packages cannot yet select patched versions themselves.
- Keep Node.js 22, pnpm 10.30.3, Next standalone output, OpenNext Cloudflare,
  Turbopack, same-origin `/mm-api`, and existing product behavior unchanged.
- Regenerate `pnpm-lock.yaml` with pnpm 10.30.3 and verify frozen installation.
- Run the production and full dependency audits against the official npm
  registry; no production high-severity finding may remain. Any full-audit
  development-tool finding must be proven incompatible with a blind override,
  bounded to trusted build inputs, and recorded explicitly.
- Run frontend format, lint, typecheck, unit tests, and production build.
- Run the full isolated-copy standalone verification because Next and build
  dependencies affect the release artifact.
- Keep generated Next/OpenNext output outside the isolated source copy so a
  preceding Worker build cannot make the source-boundary gate fail on generated
  symlinks.
- Record any remaining low/moderate advisory or compatibility exception with
  its reachability and rollback decision instead of silently ignoring it.

## Acceptance Criteria

- [x] `next` and `eslint-config-next` resolve to a patched `16.2.x` version.
- [x] `pnpm install --frozen-lockfile` succeeds.
- [x] `pnpm audit --prod --audit-level=high` succeeds against the official npm
      registry.
- [x] Full `pnpm audit --audit-level=high` is run against the official npm
      registry; any remaining high finding is limited to documented
      development-tool chains with patched compatible backports and no
      untrusted glob input.
- [x] Frontend format, lint, typecheck, 936-or-more tests, and production build
      pass.
- [x] `bash mm-chat/scripts/verify-standalone.sh --full` passes.
- [x] The dependency patch contains no API, UI, provider, storage, or runtime
      configuration behavior change.

## Definition of Done

- The dependency manifest/override/lockfile diff is reviewable and minimal.
- Audit findings and verification commands are recorded in the task result.
- Rollback is a revert of the dependency commit; no data migration is needed.
- The focused dependency change is committed without unrelated dirty files.

## Technical Approach

Use the smallest compatible parent upgrades first (`next` and
`eslint-config-next`), then update the repository's existing targeted overrides
to patched versions where upstream semver ranges still select vulnerable
packages. Do not take unrelated major upgrades. Re-audit after each lockfile
change, inspect the resolved dependency paths, and keep only overrides needed
to close an audited range.

## Decision (ADR-lite)

**Context**: A broad `--latest` refresh would mix security remediation with
major framework and SDK upgrades, while updating Next alone leaves audited
transitive findings.

**Decision**: Apply a bounded security patch with compatible direct upgrades
and narrow pnpm overrides, then prove it through audit and the full standalone
gate.

**Consequences**: The diff remains reversible and product-neutral. Overrides
must be revisited when parent packages adopt the patched versions natively.

## Out of Scope

- React, TypeScript, ESLint 10, Google GenAI 2, KaTeX, or other unrelated major
  upgrades.
- Migrating `middleware.ts` to the Next `proxy.ts` convention.
- Removing transitional Next API routes.
- CI, Voice/TTS, task-ledger, or historical documentation cleanup.

## Technical Notes

- Primary files: `mm-chat/frontend/package.json`,
  `mm-chat/frontend/pnpm-workspace.yaml`, and
  `mm-chat/frontend/pnpm-lock.yaml`.
- `mm-chat/scripts/verify-standalone.sh` excludes both `.next` and the equally
  gitignored `.open-next` build tree; neither is source or runtime state.
- Audit research: [`research/dependency-audit-2026-07-27.md`](research/dependency-audit-2026-07-27.md).
- Local default npm mirror does not implement the audit endpoint; verification
  must set `npm_config_registry=https://registry.npmjs.org`.
