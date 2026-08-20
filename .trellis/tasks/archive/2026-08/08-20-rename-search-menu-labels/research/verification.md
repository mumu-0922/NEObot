# Search Label Verification and Rollout

## Focused source verification

Per the user-selected proportional verification rule, this localized change
did not run full component or repository suites.

- Changed-file Prettier: passed.
- Targeted ESLint for `search.ts` and `searchCompatibility.test.ts`: passed.
- Focused Vitest:
  `searchCompatibility.test.ts` + `messagesParity.test.ts`: `2` files and `7`
  tests passed.
- The focused label test asserts Chinese `内置搜索`, composed Chinese
  `Tavily 搜索`, English `Built-in search`, and Japanese `内蔵検索`.

## Necessary packaging check

The Frontend Docker build completed successfully, including its required Next
production compilation and TypeScript stage. This packaging build did not
trigger unrelated full Vitest, Backend, RAG, or standalone suites.

## Live rollout

- Rollback directory: `mm-chat/backup/search-labels-20260820T080816Z`
- Retained prior image:
  `mm-chat/frontend:retained-before-search-labels-20260820T080816Z`
- New live image:
  `mm-chat/frontend:search-labels-edb11b06-20260820T080824Z`
- New live image ID:
  `sha256:79523450476933526004312f051262812c02440b695dac7896d454b7b87bf989`
- Only `mm-chat-frontend-1` changed container ID.
- Frontend health, direct Backend readiness, and same-origin Backend readiness
  passed.
- The running container's compiled artifacts contain `内置搜索`,
  `Built-in search`, `内蔵検索`, and `Tavily`, and no longer contain
  `OpenAI Web Search`.

An already-open production tab must reload before it fetches the new immutable
JavaScript assets.
