# Quality Guidelines

> Frontend verification is proportional to change risk. Prove the changed
> contract with the smallest decisive checks, and reserve full suites for
> changes whose blast radius justifies them.

## Required Commands

Run from `mm-chat/frontend/` with Node.js 22 and pnpm 10.30.3:

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm format:check
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build
```

The repository-level release gate is:

```bash
bash mm-chat/scripts/verify-standalone.sh --full
```

Select checks by scope:

- Low-risk localized copy/style changes: changed-file Prettier and ESLint (when
  TS/TSX changes) plus focused Vitest only.
- Local typed logic: changed-file format/lint, focused Vitest, and TypeScript
  typecheck when the contract is not fully isolated by the focused test.
- Shared state/API/routing, security, persistence, dependency, toolchain,
  infrastructure, or broad refactors: run the complete affected-component
  gate and any cross-layer checks.
- Run the full standalone gate for cross-layer or high-risk releases, changes
  to the standalone/deployment contract, or an explicit user request—not by
  default for every small edit.
- A production build required to package or deploy a localized UI change is a
  packaging check; it does not by itself require unrelated full test suites.

Do not claim a command passed unless it was executed.

## Formatting and Static Analysis

- Prettier is authoritative: two-space indentation, double quotes, semicolons,
  and trailing commas where Prettier emits them.
- ESLint extends Next.js Core Web Vitals and TypeScript rules.
- The current config deliberately disables `no-explicit-any` and
  `@next/next/no-img-element`; these are not evidence that arbitrary `any` or
  unsafe image behavior is acceptable. Review boundary narrowing, performance,
  and accessibility directly.
- Use the `@/*` alias for cross-area imports and keep type-only imports marked.

## Testing Requirements

- Add or update tests in `src/__tests__/` for UI, state, route, storage,
  security, and behavior changes.
- Use Vitest (`describe`, `it`, `expect`, `vi`). Tests must be deterministic and
  must not depend on live providers or the network.
- Prefer the smallest test surface that proves the contract:
  - pure helper tests, as in `anchoredPortal.test.ts`;
  - Zod boundary tests, as in `schemas.test.ts`;
  - store tests with hoisted mocks and state reset in `chatStore.test.ts`;
  - source/composition assertions for structural Next/React contracts, as in
    `chatShellA11y.test.ts` and `chatPipelineStatusBar.test.tsx`.
- Cover success, rejection/failure, stale/abort behavior, and persistence or
  migration replay when those paths change.
- Cross-layer work must trace request input -> validation -> service/store ->
  persistence/rendered result, including the error path.

## Accessibility and Security Checks

- Verify keyboard operation, focus restoration/trapping, accessible names,
  live regions, reduced motion, dark theme, and responsive/mobile safe areas
  for affected UI.
- Keep generated Markdown/HTML on the existing sanitization path.
- Never expose plaintext provider secrets, BYOK material, private chat logs, or
  user file contents through fixtures, logs, client bundles, or commits.
- Request, upload, URL, and plugin changes require boundary/limit tests; use the
  existing security helpers rather than open-coding weaker checks.

## Forbidden Patterns

- Product source or Next.js entrypoints created outside `mm-chat/frontend/`.
- `git add -f .trellis/`, broad staging, or committing runtime state/secrets.
- Disabled tests (`it.only`, `describe.only`) or placeholder assertions.
- Swallowing errors without updating UI state, returning a typed failure, or
  logging through the existing development logger where appropriate.
- Direct mutation of Zustand state or trusting raw request/storage JSON.
- New client boundaries added only to avoid understanding Server Component or
  hydration behavior.

## Review Checklist

- [ ] The file is in the existing owning area and reuses nearby abstractions.
- [ ] Server/client, local/server-mode, and persistence authority remain clear.
- [ ] External and legacy values are validated or normalized.
- [ ] Store subscriptions are narrow and async results cannot overwrite newer state.
- [ ] Loading, empty, error, abort, and cleanup paths are handled.
- [ ] Accessibility and dark/responsive behavior are preserved.
- [ ] Focused tests cover the changed contract.
- [ ] `format:check`, `lint`, `typecheck`, tests, and relevant build gates pass.
