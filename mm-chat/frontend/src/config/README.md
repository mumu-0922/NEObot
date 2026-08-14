# Configuration Modules

The `src/config` directory contains static configuration, limits, and built-in Assistant definitions. Keep this layer deterministic: configuration files should export constants, schemas, and small lookup helpers, not runtime side effects. MCP Tool definitions come from the backend catalog or administrator manifest, never frontend constants.

## Files

```text
src/config/
├── api.ts
├── assistants.ts
├── defaults.ts
├── index.ts
├── limits.ts
└── README.md
```

## Responsibilities

### `api.ts`

Defines API route paths, external service URLs, timeout values, retry settings, cache durations, and retryable status helpers. Use these exports instead of hard-coded route strings.

```typescript
import { API_ROUTES, API_TIMEOUTS } from "@/config/api";

fetch(API_ROUTES.chat.stream, { method: "POST" });
const timeoutMs = API_TIMEOUTS.default;
```

### `assistants.ts`

Defines built-in assistant metadata and assistant categories. Assistant records are product-facing presets and should remain stable enough for persisted references.

```typescript
import { BUILT_IN_ASSISTANTS, ASSISTANT_CATEGORIES } from "@/config/assistants";
```

### `defaults.ts`

Defines default model selections, chat behavior, UI options, search/RAG defaults, voice defaults, memory defaults, HTML visual prompt defaults, and system settings. These values are used when neither local settings nor server defaults provide an override.

```typescript
import {
  DEFAULT_CHAT_CONFIG,
  DEFAULT_SYSTEM_SETTINGS,
} from "@/config/defaults";
```

### `limits.ts`

Centralizes input and payload limits for chat, attachments, document parsing,
settings, and API validation. Prefer adding new limits here when the same
boundary is enforced in more than one place. Package Skill limits remain at the
typed server contract rather than the retired browser executor.

### `index.ts`

Provides the public barrel for configuration modules. Named imports from the specific module are usually clearer, but the barrel is available when a caller needs several configuration groups.

## Guidelines

- Prefer named exports over default exports.
- Keep configuration values serializable when possible.
- Keep tool descriptions and parameter descriptions in English for stable model tool-calling behavior.
- Put runtime validation in `src/lib/api/schemas.ts` or feature-specific helpers, not in config files.
- Preserve backward compatibility for exported names that are used by persisted settings or older imports.

## Adding A Route Constant

Add new routes to `API_ROUTES` in `api.ts`:

```typescript
export const API_ROUTES = {
  myFeature: {
    list: "/api/my-feature",
    detail: (id: string) => `/api/my-feature/${id}`,
  },
} as const;
```

Use the exported constant from callers instead of repeating route strings.
