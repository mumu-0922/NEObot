# Memory Tool routing adapter design

## Goals

- Reuse the selected chat Provider's normalized Tool capability instead of
  adding another hidden relevance model.
- Keep the route decision ahead of any candidate body reaching the route model.
- Make no-call and exact-call behavior deterministic, bounded, and fail closed.
- Share one Tool definition between Development capture and any future product
  Tool Loop integration.

## Non-goals

- Production hybrid-reader activation or reader promotion.
- Memory retrieval, ranking, token selection, or answer continuation.
- Candidate-aware judging or free-form relevance output.
- Provider discovery, credential storage, fallback, or model substitution.

## Data flow

```text
current request
  -> usermemory deterministic secret redaction
  -> ChatToolAdapter
       -> exact Provider/model-bound chat.ToolPlanner
       -> one search_memory definition, tool_choice=auto
       -> temperature=0, max_output_tokens=128
  -> zero Tool Calls
       -> provenance-bound UseMemory=false
  -> exactly one search_memory call with ID and explicit {}
       -> provenance-bound UseMemory=true
  -> anything else
       -> error -> usermemory no_memory
```

The route model receives only the current redacted query and Tool definition.
It receives no RRF candidate, Memory body, Memory ID, scope, revision, score, or
database authority. `usermemory` may overlap BGE work speculatively, but it
releases no hybrid final set unless the route result passes exact model and
contract provenance checks.

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| No Tool arguments | The backend already owns the current query; model-generated queries would create a second retrieval authority. | Only an explicit decoded `{}` is accepted. |
| Exact shared contract hash | A description or schema change can alter model behavior. | Adapter construction fails until the version/hash authority is intentionally updated. |
| Zero or one call only | Multiple or unknown calls are ambiguous and could broaden capability. | Missing call means abstention; every other shape fails closed. |
| Provider/model binding | GPT and DeepSeek are independent Development hypotheses. | One result cannot authorize a different configured Provider or model. |
| Normalized `chat.ToolPlanner` seam | Provider-specific JSON must not leak into Memory selection. | Official OpenAI and OpenAI-compatible Providers share one result type. |
| Official OpenAI omits `enable_thinking` | The extension is not part of the official Chat Completions schema. | The exact model, temperature, and output cap remain bound; compatible gateways still receive `enable_thinking=false`. |

## Validation and error matrix

| Condition | Result |
| --- | --- |
| Planner, Provider ID, or model ID missing | Adapter construction fails. |
| Tool JSON hash differs from the fixed SHA-256 | Adapter construction fails. |
| Query is empty after upstream redaction | Route fails and hybrid Memory is empty. |
| Provider returns no Tool Call | Valid `UseMemory=false`. |
| Provider returns one call with a non-empty ID, exact name, and explicit `{}` | Valid `UseMemory=true`. |
| Arguments are missing, `null`, malformed, or contain any key | Reject the call. |
| Call ID/name is missing or unknown | Reject the call. |
| More than one call or more than one Provider choice | Reject the response. |
| Provider error, cancellation, deadline, model drift, or contract drift | Fail closed to `no_memory`. |

## Security considerations

- Candidate content never crosses this package's Provider boundary.
- Provider errors are replaced with bounded messages; upstream response bodies
  and credentials are not returned.
- The adapter returns only a boolean plus fixed model/contract provenance.
- A Tool Call is relevance authority only. It is not user, scope, revision,
  Provider-egress, ranking, or prompt-injection authority.
- The isolated live runner supplies two independent mode-`0600` credentials:
  one for fixed SiliconFlow BGE and one for the exact route Provider. This
  package never opens those files.

## Known limitations

- The current adapter is exercised by Development capture only. A production
  answer continuation that returns bounded Memory to the same model is not yet
  wired.
- The Development prompt is the secret-redacted current query, not the full
  product conversation replay. That limitation must remain explicit in any
  live result interpretation.
- Only `openai` and `openai_compatible` route Provider types are admitted by the
  current runner profile.

## Change history

- **2026-07-29**: Added the shared `search_memory` Tool definition, strict
  normalized chat adapter, exact Provider/model binding, and fail-closed tests
  for Development-only main-model Memory routing.
