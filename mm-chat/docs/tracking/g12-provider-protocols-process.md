# G12 Provider Protocols Process

## 2026-07-20 — Decision lock

The owner requested two protocol changes only: repair the already-visible
Gemini type, retain manual OpenAI-compatible URL/Key/model discovery without
provider presets, and add Anthropic Claude as one new native protocol.

Source inspection found that the administrator configuration and Gemini Models
connection test already accepted `Gemini`, but the Go runtime Provider resolver
handled only `OpenAI` and `OpenAI Compatible`. The visible Gemini choice was
therefore a server-mode migration gap. The local nanobot reference contains 41
named registry entries, but 34 share its OpenAI-compatible backend; that
registry breadth will not be copied into this UI.

Execution is split into Gemini and Anthropic commits. Evidence, runtime state,
rollback notes, and any live-provider use will be appended after each slice.

## 2026-07-20 — G12.1 Gemini server-runtime closure

Added a Go `GeminiProvider` that resolves the configured Gemini service root to
Google's `/v1beta/openai/chat/completions` surface and reuses the hardened
OpenAI-compatible streaming, history, image, usage, and tool-planning path. The
runtime resolver now admits stored `Gemini` providers without granting OpenAI
Responses built-in Search capability.

Gemini URL handling now accepts a service root, `/v1beta`, or
`/v1beta/openai`, canonicalizes native Models requests back to
`/v1beta/models`, rejects credentials/query/fragment in the Base URL, and no
longer appends an invalid trailing `/v1`. The ordinary OpenAI-compatible manual
URL/Key path remains unchanged. The shared model-list path was moved onto the
bounded no-redirect connection-test fetcher so stored Gemini model discovery
uses `x-goog-api-key` and never leaks the key in a query string.

Verification:

```text
backend focused tests/vet                         passed
backend full tests/vet/source build               passed
backend focused race tests                        passed
frontend format/lint/typecheck                     passed
frontend tests                                    182 files / 869 tests
frontend production build                         passed
backend no-cache Compose build/readiness           passed
frontend proxy readiness                           HTTP 200
```

The source-built backend image contained non-empty API, migrate, and admin
binaries. The formal database and schema were not changed. Its two current
model-provider records are both OpenAI Compatible, so no real Google credential
was available for an authorized quota smoke; this slice records fixture-backed
wire proof rather than claiming a live Gemini call. A live smoke can be added
after an administrator saves and activates a Gemini Key through the existing
web UI.
