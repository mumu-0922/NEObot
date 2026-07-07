# Provider Flow Inventory

This inventory describes how model providers are currently resolved and how server-backed mode should change the boundary.

## Current Provider Path

Current route path:

```text
src/services/api/chatService.ts
  ↓ sends provider runtime config
src/app/api/chat/route.ts
  ↓ ChatRequestSchema.parse
src/lib/byok/server.ts::resolveProviderRuntimeConfig
  ↓
src/lib/api/chat-handler.ts
  ↓
src/lib/providers/base.ts::ProviderFactory
  ↓
OpenAI SDK or Google GenAI SDK
```

`ProviderFactory` currently owns:

- API key validation.
- Base URL normalization.
- Outbound URL safety checks.
- OpenAI client creation.
- Gemini client creation.
- Provider type switching.

## Current Provider Types

Observed provider abstractions include:

```text
OpenAI
OpenAI-compatible
Gemini
```

Additional provider metadata/model listing is handled through provider config and `/api/providers/models`.

## Server-Backed Target

In server mode, browser should not send plaintext provider secrets for ordinary hosted use. The Go backend should own provider config lookup and secret use.

Target flow:

```text
Frontend sends model/provider id
  ↓
Go backend loads provider config for user/server
  ↓
Go validates outbound URL policy
  ↓
Provider adapter streams response
  ↓
Go normalizes events/errors
  ↓
Frontend receives stable SSE contract
```

## Go Adapter Interface Draft

```go
type Provider interface {
    StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
}
```

Provider-specific logic should stay behind adapters:

```text
internal/provider/openai.go
internal/provider/openai_compatible.go
internal/provider/gemini.go
internal/provider/mock.go
```

## Security Rules

- Provider API keys are never logged.
- Provider API keys are never returned to the browser.
- Custom base URLs must pass SSRF/outbound safety checks.
- Error responses must redact provider secrets and sensitive request payloads.
- Request IDs should connect frontend error, backend log, and audit record.

## MVP Recommendation

Start with one adapter and a mock adapter:

1. Mock provider for tests and local no-secret smoke tests.
2. OpenAI-compatible adapter for broad compatibility.
3. Gemini adapter after the streaming contract is stable.

## Risks

- Existing client-side BYOK behavior may be valuable for local-first privacy. Hosted server mode should distinguish between server-managed secrets and optional BYOK envelopes.
- Provider streaming formats differ; normalize into `ChatEvent` before crossing the API boundary.
- Some current tool call behavior is executed client-side and must not be blindly moved without a sandbox design.
