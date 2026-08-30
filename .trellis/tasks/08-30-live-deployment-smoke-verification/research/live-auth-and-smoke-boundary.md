# Live Authentication and Smoke Boundary

## Runtime facts

- The active deployment uses `AUTH_MODE=required`.
- Public Login accepts only Email/Password and deliberately has no bootstrap-token compatibility route.
- The application authorizes Bearer tokens by hashing the raw 64-character token and resolving the canonical PostgreSQL `sessions` row; PostgreSQL is rechecked even when Redis contains a positive cache entry.
- The deployment environment contains no non-secret operator login credential suitable for automation, and stored passwords must not be read or rotated for a smoke test.

## Selected authentication fixture

Create one cryptographically random, short-lived Session for the existing active owner who already owns the configured live provider. The fixture:

1. generates the raw token only inside one shell process;
2. stores only `sha256(raw token)` in the canonical `sessions` table;
3. uses a unique smoke User-Agent and Session UUID;
4. never changes `users`, `user_credentials`, provider configuration, or existing Sessions;
5. calls the public Logout route after the smoke so the normal revocation and Redis invalidation path runs;
6. deletes only the exact fixture Session row after revocation and verifies absence.

This fixture is intentionally narrower than creating a temporary User: using a temporary User would not own the configured Provider, Skill, MCP, Knowledge, or Memory resources and could introduce restrictive foreign-key cleanup risk. Using an independent Session preserves resource ownership while keeping authentication state isolated and exactly removable.

## Request boundary

- Use `http://127.0.0.1:18080/mm-api` so requests traverse the deployed frontend same-origin proxy.
- Create conversations with unique `neo-live-smoke-<timestamp>` idempotency keys and titles.
- Send one persisted User Message, then one SSE Stream request with an independent idempotency key.
- Treat only a terminal `message.completed` plus a reloaded completed Assistant Message as success.
- Delete the exact temporary Conversation and verify its Message route returns `404`.
- Accept the product's soft-deleted Conversation audit row only when the active
  count is zero and all smoke-owned Skill/MCP selections and runtime artifacts
  are absent.

## Safety boundary

- Do not print raw tokens, token hashes, provider secrets, credentials, full provider descriptors, private document contents, memory contents, or Tool outputs.
- Report only status codes, counts, terminal states, durations, and boolean evidence.
- Do not rebuild/restart services or mutate Provider, Skill, MCP, Knowledge, Memory, Workspace, or Conversation selections outside smoke-owned records.
