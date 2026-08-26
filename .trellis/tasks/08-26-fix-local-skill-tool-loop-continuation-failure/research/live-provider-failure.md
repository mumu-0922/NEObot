# Live Provider failure forensics — 2026-08-26

## Persisted Turn

- Turn: `da8f2fa5-bb7f-437d-93ef-e4dd86a630ca`
- Message: `dcf00568-d9ec-459c-b0e9-b11ada177993`
- Provider/model: `PJRSVY / gpt-5.6-luna`
- Duration: 199ms
- Events: `turn.started`, three `context.injected`, `step.started`, `step.ended`,
  `assistant.message(failed)`, `turn.ended(failed)`.
- No `tool.called` or `tool.result` event exists.

This proves the request failed before any Resource, Skill, MCP, Web, Knowledge, Memory, Goal or
Workspace Tool executed.

## Direct live probe

A temporary, non-committed Go diagnostic reused the live encrypted Provider authority inside the
Backend container and invoked the real OpenAI-compatible adapter. The temporary binary and source
were removed after the probe.

| Model | Tool definitions | Result |
|---|---:|---|
| gpt-5.6-luna | 0 | HTTP 502 / `PROVIDER_UPSTREAM_FAILED` |
| gpt-5.6-terra | 0 | HTTP 502 / `PROVIDER_UPSTREAM_FAILED` |
| gpt-5.6-luna | 2 Resource only | HTTP 502 / `PROVIDER_UPSTREAM_FAILED` |
| gpt-5.6-luna | 16 old projection | HTTP 502 / `PROVIDER_UPSTREAM_FAILED` |
| gpt-5.6-luna | 17 | HTTP 502 / `PROVIDER_UPSTREAM_FAILED` |
| gpt-5.6-luna | 18 current projection | HTTP 502 / `PROVIDER_UPSTREAM_FAILED` |

The failure is independent of Tool schema and cardinality. The current loop incorrectly wraps any
first-round failure as Local Skill failure whenever the local runtime happens to be enabled.

## Safe recovery boundary

The explicit install request already has direct human authority. A supported discovery URL can be
parsed and searched deterministically before the model is called. Search/install must still delegate
to `resourceorchestrator.Service`, which delegates to `skillsupply.Service` and `mcpclient.Service`.
AIHero remains a discovery alias only; absence from the admitted Store is a bounded no-result, never
permission to fetch or execute the page.

Relevant specs:

- `.trellis/spec/backend/chat-tool-loop.md`
- `.trellis/spec/backend/resource-orchestration.md`
- `.trellis/spec/backend/agent-runtime.md`

