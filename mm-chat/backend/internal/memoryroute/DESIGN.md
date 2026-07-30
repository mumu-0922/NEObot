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

This is a separate pre-answer Provider request. It is not the same request as
the first product `ToolRoundProvider.StreamToolRound` call. That distinction is
now decisive: live Development showed that request amplification and Provider
failure dominate this adapter's results.

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
| Provider-specific thinking control | Official DeepSeek requires `thinking.type=disabled`; generic compatible gateways use `enable_thinking=false`; official OpenAI supports neither extension. | Exact-host protocol tests prevent a gateway extension from being sent to the wrong Provider. |
| Reject preflight as product architecture | `PlanTools` adds another quota-, latency-, and failure-bearing request before the answer Tool round. | Keep this adapter for failed Development evidence; integrate Memory into the existing first Tool round next. |

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
| Official DeepSeek receives generic `enable_thinking=false` | Protocol-invalid run; do not treat it as model-quality evidence. |
| Server composition attempts to install this preflight adapter | Reject; the Development profiles failed and no policy was frozen. |

## Security considerations

- Candidate content never crosses this package's Provider boundary.
- Provider errors are replaced with bounded messages; upstream response bodies
  and credentials are not returned.
- The adapter returns only a boolean plus fixed model/contract provenance.
- A Tool Call is relevance authority only. It is not user, scope, revision,
  Provider-egress, ranking, or prompt-injection authority.
- The isolated live runner supplies two independent mode-`0600` credentials:
  one for fixed SiliconFlow BGE and one for the exact route Provider. The owner
  authorized transient decrypted Server Vault copies for Development; they were
  overwritten and removed after each run. Fresh independent credentials remain
  mandatory for any future Validation run. This package opens neither the
  files nor the Vault.

## Known limitations

- The current adapter is exercised by Development capture only. A production
  answer continuation that returns bounded Memory to the same model is not yet
  wired.
- The Development prompt is the secret-redacted current query, not the full
  product conversation replay. That limitation must remain explicit in any
  live result interpretation.
- Only `openai` and `openai_compatible` route Provider types are admitted by the
  current runner profile.
- Schema v6 reports collapse most upstream route errors into
  `MEMORY_TOOL_ROUTE_FAILED`; they cannot distinguish quota, rate limiting,
  overload, transport, and protocol failures. Do not infer a subtype from the
  aggregate count.

## Development decision

```text
GPT gpt-5.6-sol
  completed/failed = 41/259
  Final Recall@5/current fact = 0.087179/0.090909

DeepSeek v4 Pro
  invalid quality evidence: wrong official thinking-control field

DeepSeek v4 Flash after protocol correction
  completed/failed = 77/223
  Final Recall@5/current fact = 0.256410/0.254545
```

No profile passed and no policy was frozen. The next implementation must make
`search_memory` one of the tools on the existing first chat model round, then
continue with bounded current-authorized results on the same Provider/model.

## Change history

- **2026-07-29**: Added the shared `search_memory` Tool definition, strict
  normalized chat adapter, exact Provider/model binding, and fail-closed tests
  for Development-only main-model Memory routing.
- **2026-07-30**: Fixed the official DeepSeek thinking-control wire contract,
  recorded failed live Development evidence, and rejected a separate
  `PlanTools` preflight for product use.
