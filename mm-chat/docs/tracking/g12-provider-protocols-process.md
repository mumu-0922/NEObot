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

## 2026-07-20 — G12.2 native Anthropic Claude

Added an `AnthropicProvider` for the native Messages API at `/v1/messages`.
It sends `x-api-key` plus the pinned `anthropic-version: 2023-06-01`, supports
SSE text deltas, bounded provider errors, token usage, system instructions,
current-branch user/assistant history, JPEG/PNG/GIF/WebP base64 image blocks,
optional Extended Thinking, cancellation, and native tool-use planning.
Thinking deltas are deliberately not emitted as answer text.
Messages and tool requests do not follow upstream redirects, so the custom
`x-api-key` header cannot be replayed to a redirect target.

The administrator provider contract now accepts `Anthropic` as a protocol and
tests stored credentials with `GET /v1/models`. Root, `/v1`, `/v1/messages`,
and `/v1/models` Base URLs normalize to one service root. The existing
encrypted browser ingress and Postgres vault remain the only credential path;
no `.env` fallback or vendor preset was added. Transitional Next/local chat,
image, code, and voice paths fail closed instead of treating Anthropic as
Gemini, while server chat routes stored Anthropic providers to Go.

Verification:

```text
backend full tests                              passed
backend focused race tests                      passed
backend go vet/source build                     passed
frontend format/lint/typecheck                  passed
frontend tests                                  182 files / 872 tests
frontend production/source image build          passed
Compose backend/frontend readiness              healthy / HTTP 200
Compose Postgres/Redis/RAG readiness             healthy
served production chunks                        Anthropic protocol present
admin provider list through Go and proxy         HTTP 200
```

The first Compose restart exposed an unrelated stale RAG image defect: its
`/app/.venv/bin/rag-worker` entry point was zero bytes, so the container exited
cleanly and restarted forever. Rebuilding `mm-chat/rag:local` from the current
source produced a 305-byte launcher; the recreated worker stayed healthy with
zero restarts. No RAG source or persisted data was changed.

The current database still contains only two OpenAI-compatible providers, so
there was no Anthropic credential to spend for a real Claude quota call. The
HTTP request/stream/tool behavior is proven against wire fixtures and the
source-built runtime; a real Models/chat/image/tool smoke remains additive when
the administrator later saves an Anthropic Key. G12 is otherwise closed.

## 2026-07-20 — G12.3 backend-managed provider runtime repair

Live browser evidence showed that user messages were persisted but every
DeepSeek stream returned HTTP 400 in one to two milliseconds, before any
provider request. The configured `FOHWSU` record, API Key, connection test, and
`deepseek-v4-pro` model were valid. A direct server-stored replay returned a
real streamed answer, isolating the failure to frontend provider identity.

`normalizeModelProvider` was dropping `isServerManaged`. Consequently the
frontend converted the selected backend record into an empty local BYOK
configuration instead of `{id:"FOHWSU",source:"server-stored"}`. The marker
is now retained, administrator provider DTO normalization is shared between
Settings and Chat, and Chat startup always reloads the authoritative provider
list before resolving the selected model. Refresh no longer depends on first
opening Settings.

The provider activation also exposed a restart-only backend defect. Knowledge
answer governance used the uppercase custom provider ID `FOHWSU` as a
processor alias even though the governance contract accepts lowercase aliases.
Custom provider governance aliases are now canonicalized to `fohwsu`; the
stored provider ID and chat `modelRef.providerId` remain `FOHWSU`.

Verification:

```text
frontend focused regression tests              25 passed
frontend format/lint/typecheck                  passed
frontend broad suite                            181 files / 872 tests passed;
                                                one unchanged child-process test
                                                blocked by sandbox EPERM
frontend Docker production build               passed
backend cmd/api focused tests                   passed
backend Docker source build                     passed
backend/frontend/RAG/Postgres/Redis             healthy
real Frontend proxy -> Go -> vault -> DeepSeek  passed
disposable proof conversation cleanup           HTTP 204
```

The real DeepSeek proof streamed `DEEPSEEK_OK` through the production frontend
proxy and used only the Postgres-vault reference; no provider Key was exposed
or copied into browser storage. The failed browser `hi` was also replayed once
against the real provider and received a persisted assistant response.
