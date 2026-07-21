# G17 Interaction Quality Process

## 2026-07-21 — Intake and slice lock

The reported failures were split into three independently testable and
committable slices: auxiliary text generation, marketplace interaction
performance, and Knowledge document bulk deletion. This prevents a broad UI
change from hiding regressions in provider routing or deletion semantics.

Initial evidence for G17.1 showed every `streamGenerateContent` caller still
used the legacy Next `/api/chat/generate` route. In server mode that route did
not resolve the Go/Postgres Server Default provider, fell back to an
unconfigured OpenAI-compatible runtime, and returned `OpenAI Compatible API key
is not configured.` The repair will reuse Go's existing runtime provider
resolver and keep auxiliary generation non-persistent.

Initial G17.2 inspection found paged lists but also large fixed
`backdrop-blur`, heavy shadows, and animation layers on marketplace modals.
Changes will be based on render/paint evidence rather than a visual redesign.

G17.3 will initially reuse the proven single-document DELETE endpoint with
bounded client concurrency. This avoids weakening the existing Knowledge
tombstone/outbox deletion boundary merely to add a convenience action.

## 2026-07-21 — G17.1 auxiliary generation closure

Added authenticated `POST /v1/chat/generate` to Go. The endpoint is
non-persistent, requires a bounded prompt and model reference, resolves the
same Server Default/server-stored/BYOK runtime provider used by chat, validates
the model through the resolved provider, consumes its stream into a bounded
text response, and returns sanitized provider failures.

The server-mode frontend API now exposes `chat.generateText`. All existing
`streamGenerateContent` consumers use it in server mode, while local mode keeps
the legacy Next route. This repairs input/message content polishing plus the
assistant, Workspace, artifact, and legacy compression helpers that share the
same function. The model-reference helper also stopped rewriting every custom
provider ID to `openai_compatible`; server-stored and BYOK provider identity is
now retained, while the virtual Server Default is mapped to the resolved
protocol family.

Verification:

```text
backend focused chat/httpserver tests                 passed
backend go vet ./... / go test ./...                  passed
frontend focused API/BYOK tests                       68 passed
frontend typecheck / lint / format                    passed
frontend full tests                                   185 files / 884 tests
frontend production build                             passed
Compose backend/frontend rebuild                      healthy / healthy
direct deployed Server Default provider generation    200, non-empty text
frontend /mm-api proxy provider generation            200, non-empty text
temporary response artifacts                          deleted
```

The two deployed smokes intentionally used the configured real provider. No
conversation, message, or other durable application row was created.
