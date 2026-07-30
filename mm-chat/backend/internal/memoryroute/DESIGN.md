# Memory Tool routing adapter design

## Goals

- Exercise the selected chat Provider's real first `ToolRoundProvider` round
  instead of adding an independent planning request.
- Keep the route decision ahead of every Memory candidate body.
- Make no-call and exact-call behavior deterministic and fail closed.
- Reuse the canonical product Tool definition and validator without a package
  cycle.

## Non-goals

- Production feature activation or reader promotion.
- Memory retrieval, final hydration, ranking, token selection, or answer
  continuation.
- Candidate-aware judging or free-form relevance output.
- Provider discovery, credential storage, fallback, or model substitution.

## Data flow

```text
current synthetic Development query
  -> ChatToolAdapter
       -> exact Provider/model-bound chat.ToolRoundProvider
       -> one canonical search_memory definition, tool_choice=auto
       -> no continuation and no preflight decoding overrides
  -> zero Tool Calls
       -> provenance-bound UseMemory=false
  -> exactly one search_memory call with ID and explicit {}
       -> provenance-bound UseMemory=true
  -> anything else
       -> error -> usermemory no_memory
```

The adapter emits a real first `ProviderRoundRequest`, but the isolated capture
does not execute the product answer continuation. Product chat separately uses
the same canonical definition and validator inside its existing multi-tool
loop, executes current-authorized retrieval after a valid call, removes
`search_memory` from later rounds, and continues on the same Provider/model.

The route model receives only the current Development query and Tool
definition. It receives no RRF candidate, Memory body, Memory ID, scope,
revision, score, or database authority. `usermemory` may overlap BGE work for
the Development lane, but releases no hybrid final unless exact model and
contract provenance pass.

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| `internal/chat` is canonical authority | Product execution must not drift from capture. | `memoryroute` delegates definition, hash, and validation. |
| No Tool arguments | The backend already owns the current query. | Only an explicitly decoded `{}` is accepted. |
| Zero or one call only | Multiple or unknown calls are ambiguous. | No call abstains; every invalid batch fails closed. |
| Provider/model binding | GPT and DeepSeek are independent hypotheses. | One result cannot authorize another Provider/model. |
| `ToolRoundProvider`, not `ToolPlanner` | Schema-v6 proved a separate preflight amplifies latency, quota, and failures. | Schema-v7 uses the existing streaming first-round protocol. |
| No forced decoding overrides | Product first-round behavior includes ordinary selected-model settings. | Schema-v7 omits the old temperature/output/thinking profile fields. |
| Development adapter remains separate from product composition | Capture needs a boolean decision without running chat. | Server code uses `internal/chat` directly; this adapter grants no activation authority. |

## Validation and error matrix

| Condition | Result |
| --- | --- |
| Provider, Provider ID, or model ID missing | Adapter construction fails. |
| Canonical Tool JSON hash differs from the fixed SHA-256 | Adapter construction fails. |
| Query is empty | Route fails and hybrid Memory is empty. |
| Provider returns no Tool Call | Valid `UseMemory=false`. |
| Provider returns one call with a non-empty ID, exact name, and explicit `{}` | Valid `UseMemory=true`. |
| Arguments are missing, `null`, malformed, or contain any key | Reject the call. |
| Call ID/name is missing or unknown | Reject the call. |
| More than one call or an invalid/unknown event | Reject the response. |
| Provider error, cancellation, deadline, model drift, or contract drift | Fail closed to `no_memory`. |
| Caller expects answer continuation from this adapter | Reject the assumption; continuation belongs to product chat. |

## Security considerations

- Candidate content never crosses this package's Provider boundary.
- Provider errors are replaced with bounded messages; upstream bodies and
  credentials are not returned.
- The adapter returns only a boolean plus fixed model/contract provenance.
- A Tool Call is relevance authority only. It is not ownership, scope,
  revision, Provider-egress, ranking, or prompt-injection authority.
- Live capture still requires two independent mode-`0600` credentials and
  project-scoped teardown. This package opens neither credential files nor the
  Server Vault.

## Known limitations

- The Development request contains one synthetic current query, not the full
  product conversation replay.
- The adapter measures only the first-round route decision. It does not measure
  the product continuation response.
- The current live runner admits only `openai` and `openai_compatible` route
  Provider types.
- No schema-v7 live Development evidence exists; offline fake-protocol results
  are lifecycle evidence only.

## Evidence separation

```text
schema-v6 / profile-v6 / cost-basis-v4
  -> immutable failed PlanTools-preflight evidence

schema-v7 / profile-v7 / cost-basis-v5
  -> first ToolRound adapter/profile
  -> offline protocol gates passed
  -> live Development not run
  -> Validation and Promotion blocked
```

## Change history

- **2026-07-29**: Added the original normalized main-model Memory route for
  Development capture.
- **2026-07-30**: Corrected official DeepSeek thinking control, recorded failed
  schema-v6 live evidence, and rejected `PlanTools` preflight architecture.
- **2026-07-30**: Moved canonical Tool authority to `internal/chat`, changed the
  Development adapter to the real first `ToolRoundProvider` request, and added
  schema-v7 offline evidence separation.
