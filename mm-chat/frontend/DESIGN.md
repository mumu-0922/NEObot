# mm-chat Frontend Adapter Scaffold Design

## Design Goals

- Keep Phase 11.1A isolated under `mm-chat/` until original app `src/` wiring is approved.
- Define a compile-safe `local|server` API-client shape that later maps into `src/services/api/client/*`.
- Make rollback safe by resolving missing, invalid, or unconfigured modes to `local`.
- Centralize server URL building, JSON error envelope handling, and Go SSE frame parsing.
- Fail closed for unsupported features instead of silently falling back to browser-local persistence in server mode.

## Non-Goals

- No React, Zustand, localforage, OPFS, or `ChatApp` wiring in this slice.
- No conversation/message CRUD calls to Go yet.
- No live assistant stream integration yet.
- No file upload/download UI integration.
- No auth, import/export, RAG, plugin, voice, document, image, or provider-settings migration.

## Architecture

```text
consumer (future src/services/api/client)
  -> createNeoChatApiClient(config)
  -> mode resolver
  -> local chat shell OR server chat shell
  -> shared HTTP/SSE helpers
```

Current files:

- `src/api-client/types.ts` defines DTOs and adapter contracts.
- `src/api-client/mode.ts` resolves mode, base URL, capabilities, and browser network edge.
- `src/api-client/errors.ts` defines normalized adapter errors.
- `src/api-client/server/http-client.ts` builds server URLs and normalizes JSON/network failures.
- `src/api-client/server/sse.ts` parses Go named SSE frames.
- `src/api-client/local/chat-api.ts` and `src/api-client/server/chat-api.ts` are explicit unsupported shells.
- `__tests__/api-client.test.ts` covers resolver, HTTP helper, and SSE parser behavior.

## Key Decisions

| Decision                                         | Reason                                                                | Tradeoff                                                                       |
| ------------------------------------------------ | --------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| Keep code under `mm-chat/frontend/`              | Preserves the owner constraint not to edit the existing app casually. | Does not activate server mode in the current UI yet.                           |
| Default invalid or unconfigured modes to `local` | Safe rollback must not depend on backend availability.                | Misconfiguration is surfaced as warnings rather than hard failure.             |
| Treat missing server base URL as local fallback  | Prevents accidental browser calls to an empty or wrong origin.        | Operators must inspect warnings during server-mode smoke.                      |
| Parse SSE with `fetch`-compatible framing        | Go stream requires POST body, which native `EventSource` cannot send. | Full streaming consumption is deferred to Phase 11.3.                          |
| Unsupported methods fail closed                  | Avoids hidden browser-local persistence in server mode.               | Callers must handle unsupported results until later slices implement behavior. |

## Security Considerations

- Browser-visible contracts must not include provider API keys, local secret envelopes, object-store keys, bucket names, local paths, or MinIO/S3 URLs.
- Direct backend URLs require explicit CORS allowlisting before browser smoke; otherwise use a same-origin proxy/reverse proxy.
- Network/CORS/timeout failures are normalized as recoverable adapter errors.
- SSE event names must match `data.type`; mismatches fail closed as `STREAM_PROTOCOL_ERROR`.
- This scaffold does not read `.env.single-server`, `backend/.env`, or any real secret file.

## Known Limitations

- The current Next.js app still does not import this scaffold.
- `phase11Capabilities` are all disabled until later slices wire real behavior.
- The local and server chat adapters expose shells only; CRUD, stream, cancel, and files are future slices.
- The type-check command verifies the scaffold entrypoint, not the entire root app, because local dependencies are not installed in this environment.

## Verification

```bash
corepack pnpm dlx vitest@4.1.9 run mm-chat/frontend/__tests__/api-client.test.ts
corepack pnpm --package=typescript@5.9.3 dlx tsc --noEmit --target ES2020 --module ESNext --moduleResolution Bundler --lib DOM,ESNext --strict --skipLibCheck mm-chat/frontend/src/api-client/index.ts
corepack pnpm dlx prettier@3.9.4 --check 'mm-chat/frontend/**/*.ts' mm-chat/frontend/README.md
git diff --check -- mm-chat
```

## Change History

- 2026-07-08: Added isolated Phase 11.1A adapter scaffold under `mm-chat/frontend/`.
