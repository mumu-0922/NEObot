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

## Deterministic Browser E2E

Use Playwright for high-risk journeys that must prove the integrated browser
contract rather than an isolated component contract:

```bash
corepack pnpm test:e2e:install
corepack pnpm test:e2e
```

- Keep browser specs in `e2e/` and shared API fixtures in `e2e/fixtures/`.
- Keep Vitest discovery restricted to `src/__tests__/`; Playwright `*.spec.ts`
  files must never be loaded by the unit-test runner.
- Exercise the production UI and `/mm-api` request shapes. Do not add test-only
  branches, routes, query parameters, or globals to product components.
- Fixture state is server-authoritative and must survive `page.reload()` when
  the production contract is durable. Each test creates its own fixture state
  so the suite remains parallel-safe.
- Never read a real Provider key or spend Provider quota. Return deterministic
  HTTP/SSE payloads from the browser route fixture and use inert credentials.
- Prefer role, label, and visible-text locators. Use a stable id only where the
  user-facing contract cannot identify the target.
- Cover both the happy path and decisive terminal failures: unauthorized,
  failed, cancelled, stale/reloaded, and independently concurrent runs where
  applicable.
- RAG fixtures must follow the current authority chain: persist
  `config.selectedKnowledgeCollectionIds` through the Conversation PATCH/read
  contract, then return terminal `metadata.knowledge` on the assistant
  message. Do not require the stream request to repeat the selection in
  metadata; that request field is a legacy migration fallback because the
  Backend resolves the current binding from the Conversation.
- A cited RAG answer is valid only when both the assistant metadata contains
  the citation and the answer contains its issued marker (for example `[K1]`).
  Browser coverage must expand the citation card and assert its bounded source
  projection. The decisive failure case must return a terminal degraded
  outcome with zero citations rather than leaving a spinner running.
- Memory browser fixtures must preserve Server authority across refresh:
  governance snapshots and policy mutations come back through `/mm-api`, not
  IndexedDB or a product E2E branch. Recall is represented by the durable
  `search_memory` process-step contract, not by inspecting prompt text.
- Direct Memory action coverage must bind Activity to the exact terminal
  assistant Message, poll through the production Activity API, and send the
  Activity `subjectRevision` on undo. Seed terminal Activity before resolving
  fixture SSE so the test never depends on polling sleeps.
- Skill/MCP browser fixtures must keep Library/inventory and revision-bound
  per-Conversation selections as separate server authorities. Installing a
  Skill changes only Library inventory; it must not seed any Conversation
  selection. Selection fixtures must reject unknown installation/Server IDs.
- Resource execution coverage must project schema-version-1 Skill/MCP cards
  from durable Transcript v2 events, assert interleaved narration/Tool list
  items through semantic locators before the final answer, and replay the same
  order after reload. A failed MCP step must reach a terminal failed state and
  remove the Conversation running indicator.
- CI runs Chromium separately from Vitest and retains trace, screenshot, and
  video artifacts only on failure.

Wrong:

```ts
await page.locator(".button:nth-child(3)").click();
await callLiveProvider(process.env.REAL_PROVIDER_KEY);
```

Correct:

```ts
await page.getByLabel("发送消息").click();
api.completeRun(conversationId, "deterministic result");
```

For durable Resource order, assert the semantic process blocks rather than a
single asynchronous `textContent()` snapshot:

```ts
const blocks = message
  .getByRole("region", { name: "Agent 执行过程" })
  .getByRole("listitem");
await expect(blocks.nth(0)).toHaveText("先运行已选择的 Skill。");
await expect(blocks.nth(1)).toContainText(skillName);
await expect(blocks.nth(2)).toHaveText("再调用已选择的 MCP。");
await expect(blocks.nth(3)).toContainText(serverAndToolName);
```

For RAG selection, assert the persisted Conversation fixture state instead of
the retired request duplication:

```ts
expect(conversation.config?.selectedKnowledgeCollectionIds).toEqual([
  collectionId,
]);
api.completeKnowledgeRun(conversationId, "grounded answer [K1]", {
  outcome: "answered",
  citations: [{ id: "c1", marker: "[K1]", snippet: "bounded evidence" }],
});
```

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
