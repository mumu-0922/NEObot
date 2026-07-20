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

## 2026-07-20 — G12.4 server image routing and terminal failure repair

The owner-visible image card continued counting past three minutes even though
Go had failed the request in 9 ms. Runtime and Postgres evidence showed
`openai_compatible:gpt-image-2`, `IMAGE_EXECUTOR_RESOLUTION_FAILED`, and an empty
assistant row with `status=failed`. The image resolver understood only
`SERVER_DEFAULT` or an exact stored provider record ID, while the frontend
intentionally retained the legacy `openai_compatible` protocol identity for
chat/RAG compatibility. Reload mapping then discarded the failed status and
error metadata, so the empty image assistant looked pending forever.

The resolver now maps the three legacy OpenAI aliases to Server Default only
when its connection-tested model list contains the exact requested image model.
Custom record IDs still resolve only their own Postgres/vault record. Resolved
provider DTOs now carry their model allowlist so that compatibility fallback is
bounded rather than unconditional.

The migrated Go chat path also differed from the previously proven image call:
it omitted the chat size and did not request base64 output. Chat image dispatch
now sends `1024x1024`, and the OpenAI-compatible executor sends
`response_format: b64_json` while retaining URL-response compatibility. Safe
failure classification logs only stage/status categories, never provider
bodies, prompts, signed image URLs, credentials, or bytes.

Frontend DTO reload now maps failed assistant rows and `metadata.errorCode` to
`generationError`. Live failed stream results apply the same terminal error to
the assistant draft, and `MessageItem` explicitly excludes generation errors
from loading inference. A refresh can no longer resurrect the timer.

Verification:

```text
backend full tests / vet                              passed / passed
frontend tests                                       182 files / 874 tests
frontend lint / typecheck / format                   passed / passed / passed
backend/frontend production source builds            passed / passed
backend/frontend/Postgres/Redis/RAG                   healthy
real gpt-image-2 chat SSE                             message.started -> message.completed
real image artifact                                  image/png, 2,407,661 bytes
real stream duration                                 88,843 ms
negative unconfigured image model                    message.error in 8 ms
positive file/conversation cleanup                   HTTP 204 / HTTP 204
negative conversation cleanup                        HTTP 204
```

The provider produced transient failed/timeout attempts during the authorized
smoke before one complete response; those attempts terminated correctly and
their conversations were deleted. The final positive request was persisted,
downloaded, byte-checked, and then removed from file storage and chat state.

## 2026-07-20 — G12.4.1 image policy failure classification

The owner-visible `画个海绵宝宝` failure was replayed against the active
provider. The upstream returned HTTP 400 with the machine code
`content_policy_violation`; model selection, Server Default resolution, Key,
request delivery, and artifact storage were not the failing stages. A
SpongeBob-like indirect description was rejected the same way, while an
unrelated original landscape completed normally. Provider moderation is
therefore the decisive boundary, not an intermittent frontend timeout.

The executor now parses only bounded, allowlisted provider `code`/`type` labels,
maps every unknown label to `UNRECOGNIZED`, discards the provider message/body,
and records a sanitized failure category. Transient network, `408`, `429`,
`5xx`, read/decode/empty-response, and URL-fetch failures receive at most one
retry inside the original request deadline. HTTP 400 content-policy rejection,
invalid base64, oversized data, and invalid URLs are permanent failures and are
not retried.

Go maps the policy rejection to terminal SSE/database code
`IMAGE_CONTENT_POLICY_VIOLATION`. The frontend preserves that code across
reload, marks it non-recoverable, and renders localized Chinese, English, or
Japanese guidance instead of the sanitized English backend message. A provider
that exhausts the shared request deadline maps separately to recoverable
`IMAGE_PROVIDER_TIMEOUT` with localized retry guidance.

Verification:

```text
backend full tests / vet                              passed / passed
frontend tests                                       182 files / 876 tests
frontend lint / typecheck / format                   passed / passed / passed
backend/frontend production source builds            passed / passed
backend/frontend/Postgres/Redis/RAG                   healthy
real named-character prompt                          IMAGE_CONTENT_POLICY_VIOLATION, 68,749 ms
real indirect character prompt                       IMAGE_CONTENT_POLICY_VIOLATION, 66,772 ms
real original landscape prompt                       image/png, 3,285,229 bytes, 75,710 ms
post-rebuild named-character replay                   IMAGE_PROVIDER_TIMEOUT, 120,008 ms
final-build named-character replay                    image/png, 2,283,680 bytes, 79,105 ms
all smoke conversations/files                        HTTP 204 cleanup
```

No prompt, provider body, URL, credential, or image bytes were written to the
application log. Rollback is the single follow-up commit; reverting it restores
the generic provider error but must not revert the preceding G12.4 image-route
repair.

## 2026-07-20 — G12.4.2 provider connection failure classification

The owner-visible 17:50 request for a Corgi/brand collaboration poster did not
reach an HTTP provider response. The production log correlated the browser
message append at `09:50:08Z` with terminal
`IMAGE_PROVIDER_REQUEST_FAILED` at `09:51:55Z`; the stream lasted 106,583 ms.
This category is emitted only after the executor's single bounded retry also
fails at the request transport stage. There is no response status or provider
policy code for this attempt, so the evidence does not support attributing this
specific failure to prompt moderation.

Request, response-read, generated-image-fetch, and fetch-read transport
failures now map to terminal chat code `IMAGE_PROVIDER_CONNECTION_ERROR` after
retry exhaustion. Frontend live/reload rendering retains the code as
recoverable and shows localized Chinese, English, or Japanese connection
guidance instead of the generic English provider error. Raw transport errors,
provider URLs, prompts, credentials, and response bodies remain excluded from
logs and SSE.

Verification:

```text
backend full tests / vet                              passed / passed
backend focused race tests                            passed
frontend tests                                       182 files / 876 tests
frontend lint / typecheck / format                   passed / passed / passed
backend/frontend production source builds            passed / passed
backend/frontend readiness and frontend proxy health healthy / passed
live owner request correlation                       IMAGE_PROVIDER_REQUEST_FAILED, 106,583 ms
```

The already-failed row retains its historical generic error code; new attempts
receive the classified code. Rollback is the G12.4.2 commit only and does not
alter provider credentials, model selection, persisted images, or schema.

## 2026-07-20 — G12.4.3 GPT Image SSE timeout closure

The earlier synchronous probes still establish that the calling chain lost its
connection near 60 seconds, but they did not establish an OpenResty read
timeout. Owner-side SSH evidence corrected that attribution:

```text
active root.conf proxy_connect_timeout / send_timeout / read_timeout  10s / 600s / 600s
active root.conf send_timeout                                           600s
nginx -t                                                                passed
failed request at OpenResty                                             499
sub2api completion one second later                                     200 + broken pipe
```

The 600-second site configuration was loaded by the active worker. `499` means
OpenResty observed its client disconnect first; `keepalive_timeout 60` is an
idle post-response setting and is not the upstream response deadline. The old
claim that this route needed a higher OpenResty timeout is superseded.

The [official Image generation guide](https://developers.openai.com/api/docs/guides/image-generation)
defines `stream: true` with `partial_images: 0..3`; GPT Image complex prompts
may take up to two minutes. One requested partial image costs an additional 100
image output tokens. An isolated Vault-backed request to the configured relay
proved that its `gpt-image-2` endpoint implements this SSE contract without
exposing the Key, prompt response, URL token, or image bytes:

```text
HTTP/2 200 text/event-stream headers       12,821 ms
image_generation.partial_image index=0     42,835 ms
image_generation.partial_image index=1     78,228 ms
image_generation.completed                 78,581 ms
EOF                                         78,581 ms
```

The Go image executor now selects SSE only for `gpt-image-*`, sends
`partial_images: 1`, omits the legacy `response_format`, and accepts both relay
shapes: an image-bearing completed event or the last image-bearing partial
before an empty completion marker. Other OpenAI-compatible image models keep
the existing synchronous JSON contract. Partial frames only keep the upstream
connection active; they are never persisted or exposed as separate chat files.

Debug retrospective: this was a cross-layer contract and implicit-assumption
failure, amplified by a missing deployed integration test. Raising only local
deadlines and varying synchronous payload fields treated symptoms; attributing
`499` to OpenResty treated the observer as the failing caller. Prevention is now
executable: verify loaded proxy state and status ownership, measure SSE headers
and events, regression-test both terminal event shapes, and finish with a real
container-to-provider artifact/cleanup proof. The reusable checklist is in
`.trellis/spec/guides/cross-layer-thinking-guide.md`.

Verification after rebuilding only the Backend container:

```text
backend full tests / vet                              passed / passed
backend focused race tests                            passed
backend/frontend/Postgres/Redis/RAG                   healthy
real complex Corgi collaboration poster               message.started -> message.completed
real stream duration                                  67,327 ms
real final artifact                                   image/png, 2,235,881 bytes
positive file/conversation cleanup                    HTTP 204 / HTTP 204
backend terminal provider failure                     absent
```

The temporary diagnostics were deleted, the final smoke file and conversation
were deleted, and Server Default/Postgres/Vault configuration was not changed.
Rollback is the G12.4.3 implementation commit; reverting it restores
synchronous GPT Image calls and therefore restores the idle-disconnect risk.

## 2026-07-20 — G12.5 model-aware reasoning effort

The previous composer exposed only a boolean. Go translated every enabled
OpenAI-compatible request to `reasoning_effort: high`, OpenAI Responses used
`reasoning.effort: high`, and Anthropic always used a 4,096-token thinking
budget. Users could not choose the quality/latency tradeoff.

Kelivo commit `92db9a4` was inspected at its mobile/desktop reasoning budget UI,
OpenAI model compatibility matrix, and provider request adapters. Its reusable
decision is one semantic user setting with provider-specific translation, not
one raw token budget forwarded to every API. The current
[OpenAI reasoning guide](https://developers.openai.com/api/docs/guides/reasoning)
also confirms that supported effort values are model-dependent and may include
`none`, `minimal`, `low`, `medium`, `high`, `xhigh`, and `max`.

mm-chat now exposes Off, Auto, Low, Medium, and High for reasoning-capable
models. Known GPT-5.2+ and DeepSeek families additionally expose XHigh; GPT-5.6
also exposes Max. The selection is persisted in the server conversation config
and the browser store so reload and new-chat defaults remain coherent. Existing
`useReasoning=true` records without a level retain High.

The Go boundary accepts only `auto|low|medium|high|xhigh|max`. OpenAI-compatible
Chat Completions and OpenAI Responses receive model-normalized effort; Auto
omits the effort field. Unknown compatible models clamp XHigh/Max to High,
GPT-5.4 Max clamps to XHigh, and GPT-5.6 retains Max. Anthropic maps the same
levels to 1,024/2,048/4,096/8,192/16,384 thinking tokens and raises
`max_tokens` when necessary so the thinking budget is always lower.

Verification:

```text
backend full tests / vet                              passed / passed
backend focused race tests                            passed
frontend tests                                       183 files / 879 tests
frontend lint / typecheck / format                   passed / passed / passed
backend/frontend production source builds            passed / passed
backend/frontend/Postgres/Redis/RAG                   healthy
real gpt-5.6-sol Low                                  completed, 2,587 ms
real gpt-5.6-sol Max                                  completed, 2,000 ms
real Low/Max conversation cleanup                     HTTP 204 / HTTP 204
headless UI options                                   Off through Maximum
headless Maximum selection + reload                   persisted / restored
post-smoke owner conversation setting                 restored to Off/High
```

No provider Key, response text, reasoning text, or private message content was
recorded. Rollback is the G12.5 implementation commit; no schema or provider
secret migration is involved.
