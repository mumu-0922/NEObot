# Fixed Memory Judge transport-stability follow-up

## Scope and evidence boundary

This review was performed on 2026-08-04 after the single authorized schema-v13
Development diagnostic. It uses only the retained aggregate report, checked-in
source, and pinned public open-source implementations. It makes no Provider
request and does not authorize a rerun, Validation, production activation, or
promotion.

The retained evidence is intentionally identity-free. It cannot establish
which case or cases retried, whether the three failed attempts belonged to one
or two logical requests, or which private network error occurred.

## Exact schema-v13 evidence

Run `memory-regression-20260804t005257z-8f43c5e7` completed all 300 Development
cases under configuration
`f1971a3fabc93149170b216d440998b73e1d5c40f277b1b41c574bcd72016579`.
The diagnostic report is complete and reconciled:

```text
empty candidate cases       = 105
Judge-completed cases       = 194
failed cases                = 1
CANDIDATE_JUDGE_FAILED      = 1
Judge attempts              = 197
Judge retries               = 2
attempt failure categories  = PROVIDER_STREAM_READ_FAILED: 1
                              PROVIDER_TRANSPORT_FAILED: 2
terminal failure categories = PROVIDER_TRANSPORT_FAILED: 1
```

The aggregate equations all hold:

```text
1 terminal category = 1 CANDIDATE_JUDGE_FAILED
3 attempt failures  = 2 retries + 1 terminal attempt failure
197 attempts        = 194 completed + 1 terminal failure + 2 retries
105 + 194 + 1       = 300 cases
```

No JSON, schema, ordinal, provenance, Recorder, HTTP status, context, or
deterministic adapter category occurred. Quality evaluation independently
passed with Candidate Recall@20 `1.0`, Final Recall@5 `0.9948717949`,
current-fact accuracy `0.9939393939`, false-injection rate `0`, and every
safety counter zero. The top-level report remains permanently non-passing and
non-selecting because schema v13 is diagnostic-only.

The report SHA-256 is
`381df1eb72c29bf4a6a478731797250998cdc58482becaa44bf0b9abfef58527`;
the run-manifest SHA-256 is
`cff8b7408841939e530a53aacb98f1894c2c7cf797bf4124a52f6c64f86284a3`.

## Local transport behavior

The fixed Judge currently executes this path:

```text
memoryjudge.ChatAdapter
  -> chat.Provider.StreamChat
  -> OpenAI-compatible POST /chat/completions with stream=true
  -> SSE scanner
  -> AccuracyFirstProviderController
       transient 408/429/5xx/transport/read retry once
       fixed five-second fallback delay
```

The controller globally serializes BGE and Judge calls and the runner inserts
a real one-second inter-case cooldown. WSL/Docker pressure therefore does not
justify more concurrency. The current weakness is retry depth: one retry can
recover one transient failure, but the diagnostic retained one transport
failure after that ceiling was exhausted.

Changing the prompt, BGE models, corpus, `0.02` gate, HTTP/2 setting, connection
reuse, or response mode is not supported by this aggregate. In particular, one
stream-read failure does not prove that SSE caused the two transport failures.

## Open-source transport comparison

The comparison is pinned to these public commits:

| Project | Inspected commit |
| --- | --- |
| OpenAI Go | [`f490c006504831df5077620eae5f43d87759b586`](https://github.com/openai/openai-go/tree/f490c006504831df5077620eae5f43d87759b586) |
| Anthropic Go | [`0303a8539676836e0cb351f3489fc2d347bbacde`](https://github.com/anthropics/anthropic-sdk-go/tree/0303a8539676836e0cb351f3489fc2d347bbacde) |

OpenAI Go retries connection failures, `408`, `409`, `429`, and `5xx` twice by
default. Its fallback delay is capped exponential backoff with downward jitter,
while valid `Retry-After-Ms` or `Retry-After` is authoritative. It refuses a
retry when the request body is not replayable and keeps the caller context
authoritative across the whole retry sequence. Relevant sources are
`README.md` under **Retries** and
`internal/requestconfig/requestconfig.go` functions `shouldRetry`,
`parseRetryAfterHeader`, and `retryDelay`.

Anthropic Go independently defaults to two retries and uses the same bounded
request-option shape. Its longer-lived stream loops also reconnect transient
stream failures with capped jittered backoff while terminating deterministic
4xx failures. Relevant sources are `option/requestoption.go` and the session
stream runner.

Useful pattern: two bounded retries for typed transient failures, exact
`Retry-After` precedence, replayable request bodies, one caller-owned context,
and no retry for deterministic protocol/output errors.

Do not copy blindly: Neo Chat has global Provider concurrency one, so
fleet-level jitter has no thundering-herd benefit inside this isolated run.
Its deterministic fake-protocol evidence also benefits from an exact declared
fallback schedule. The current five-second first fallback is more conservative
than either SDK and should not be shortened without route-specific evidence.

## Recommended repair

Create a separately versioned accuracy-first transport-stable Development lane;
do not mutate schema v12 or v13:

1. Keep the exact BGE tuple, Luna Provider/model, prompt v1, strict decoder,
   selection algorithm, criteria v3, global concurrency one, one-second
   inter-case cooldown, no elapsed deadline, and fail-closed behavior.
2. Permit at most **two retries** for each Judge logical request while keeping
   the existing typed retryable set. Use exact fallback delays of five seconds
   then ten seconds; honor a valid non-negative `Retry-After` instead.
3. Keep retrieval Provider requests at the historical one-retry ceiling. The
   observed terminal failure is Judge-specific and does not justify amplifying
   every BGE call.
4. Add a new execution-policy/profile/reader/report identity and new cost-basis
   authority covering the worst-case three Judge attempts. Preserve v12/v13
   bytes and validation rules exactly.
5. Retain the schema-v13 attempt/terminal taxonomy in process during offline
   tests. A future selecting report may publish only the existing aggregate
   failure totals; it must fail closed if any Judge request remains terminal.
6. Prove recovered first/second retry, exhausted retry, deterministic no-retry,
   caller cancellation, exact wait schedule, telemetry reconciliation, maximum
   request/token/cost ceilings, fake-protocol byte replay, and teardown before
   asking for any live authority.

Do not switch to non-streaming or required Tool output in this repair. Those are
separate hypotheses with wider adapter/protocol changes. Reconsider
non-streaming only if a transport-stable diagnostic still shows stream-read or
stream-incomplete failures as the dominant terminal category.

## Authorization boundary

The schema-v13 run consumed its one-run authority. A transport-stable live run
would require a new exact cost document, fresh explicit owner authorization,
independent temporary credentials, and the same global/Compose concurrency of
one. No automatic rerun is permitted.
