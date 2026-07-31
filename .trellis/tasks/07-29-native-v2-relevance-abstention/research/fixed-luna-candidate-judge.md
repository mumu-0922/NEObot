# Fixed Luna candidate-judge successor

## Decision

The owner rejected a local candidate-aware model and selected one globally
fixed cloud Memory Judge independent of the active answer model:

```text
provider ID    = SERVER_DEFAULT
provider type  = openai_compatible
base URL       = https://sub.mumubuku.top/v1
base URL SHA   = 3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671
model alias    = gpt-5.6-luna
adapter        = chat-configured-candidate-judge-v1
prompt         = memory-cloud-candidate-judge-prompt-v1
prompt SHA     = c004e834f2db572fc8393f088f47750d420379664f972357f987a09d8647f9c8
decoding       = temperature-0_max-output-128_no-thinking_v1
```

The intermediary alias does not prove the upstream model implementation or
parameter count. The evidence and future profile bind only this observable
Provider/Base-URL/model/adapter identity and live behavior.

## Version boundary

Schema v10 is immutable failed evidence for the configured
`gpt-5.6-sol` and `deepseek-v4-flash` profiles. The Luna successor must use new
schema-v11 profile, report, criteria, and cost identities rather than changing
schema-v10 constants or reinterpreting its reports.

The owner-selected schema-v11 complete-flow latency budgets are:

```text
p95          <= 1500 ms
p99          <= 2500 ms
hard cutoff  <= 3000 ms
```

These budgets cover candidate recall, authorization/redaction, concurrent BGE
rerank and cloud judging, intersection, and final reauthorization. They are a
product decision, not an industry-standard claim. Historical schema-v10
`900/1500/2000 ms` evaluation remains unchanged.

## Protocol smoke

One authorized request used the existing strict `memoryjudge.ChatAdapter` and
the active Server Vault credential. The synthetic input contained one directly
useful caffeine fact, one unrelated preference, and one candidate-body prompt
injection. The exact adapter sent `temperature=0`, `enable_thinking=false`,
`max_tokens=128`, SSE chat completions, and the unchanged strict system/user
prompt.

Sanitized result:

```text
contract pass       = true
semantic smoke pass = true
selected ordinals   = [0]
elapsed             = 2354 ms
hard cutoff         = 3000 ms
```

This proves the intermediary accepts the current OpenAI-compatible request and
the response passes the exact ordinal JSON decoder. It does not prove the
provider's internal implementation, p95/p99 latency, 300-case quality, or
promotion eligibility. The single judge-only latency is already above the
future p95 target, so full-flow latency is a high-risk hypothesis that must be
measured rather than assumed.

## Credential and retention boundary

- The smoke read only the actively configured `SERVER_DEFAULT` envelope and
  active keyring referenced by `.env.single-server`.
- Plaintext credentials never entered argv, environment variables, logs,
  output, Git, or retained artifacts.
- Raw query/response data was not retained; only the synthetic fixture and
  sanitized result above remain.
- The transient encrypted-envelope copy was mode `0600` and shredded; the
  temporary Go helper was removed.

The full Development runner must continue to receive an independent temporary
mode-`0600` credential file. It must not gain Vault decryption authority.
The owner authorized an operator-only export from the existing
`SERVER_DEFAULT` Vault record for schema-v11 Development. The copy must be
created only after offline gates pass, mounted read-only into the isolated
runner, scanned out of every retained surface, overwritten, and removed on
success, failure, or interruption. This authorization does not cover
Validation or production activation.

The owner separately authorized the same one-run export boundary for the
existing `RAG:SILICONFLOW` credential used by fixed BGE-M3 embedding and
reranking. The Luna and BGE files remain separate credential authorities: mode
`0600`, read-only in the runner, rejected when hard-linked or byte-equal, and
destroyed on every exit path. Neither runner receives Vault decryption access.

The owner conditionally authorized one real 300-case schema-v11 Development
run after all offline, race, PostgreSQL, Compose, and secret-scan gates pass.
That authorization covers Luna and SiliconFlow quota for Development only. A
failed gate stops the line without retuning; Validation and production remain
unauthorized.

## Schema-v11 outcome and schema-v12 successor

The authorized schema-v11 run completed and retained report SHA-256
`0dfe7733005bd211664ebaa47a9a5325c0638288f90c736986756eda34a37205`.
It failed unchanged quality and latency gates: only `41` Luna requests were
attempted, only `22` cases completed both rerank and judge, `154` cases recorded
`RELEVANCE_ADMISSION_UNAVAILABLE`, and `19` complete stages recorded
`HARD_CUTOFF`. That report remains immutable criteria-v2 evidence. The one-run
authorization and transient credentials were consumed and destroyed; they do
not authorize another live run.

The owner selected a separately versioned accuracy-first schema-v12
Development profile. It preserves the fixed Luna/BGE/prompt/decoder/egress
authority but executes query embedding, local admission, BGE rerank, Luna
judge, and Record serially under global Provider concurrency one. It removes
application and HTTP elapsed deadlines, makes latency diagnostic-only, waits
one second between cases, and permits one retry only for 408/429/5xx or a
retryable transport/read interruption. Fake mode uses a virtual cooldown;
live mode uses wall-clock sleep. Cost-basis v8 pre-authorizes at most 600 Judge
attempts and 76800 output tokens and reconciles actual input/output authority
from aggregate attempt telemetry.

## Remaining work

1. Schema-v12 focused race, all-backend, `go vet`, regression-topology,
   PostgreSQL 17 `fake_protocol`, frontend, RAG, secret-scan, and decomposed
   standalone checks passed on 2026-07-31. The monolithic full-standalone
   command was interrupted twice by a Docker Desktop WSL integration proxy
   crash after its structure/topology checks passed; this is retained as an
   infrastructure limitation, not reported as a full-gate pass. The fake run
   retained the expected private two-file failed-metric bundle, recorded 300
   query attempts, 195 serial rerank-plus-judge decisions, 299 virtual
   cooldowns, zero retries, and destroyed every isolated runtime object.
2. Do not make a live schema-v12 Provider call without a fresh, separate
   Development authorization and new temporary credentials.
3. Even if Development passes, stop again. Validation and production remain
   separate future authorizations.

Even when every Development gate passes, the runner must stop and present the
aggregate report for owner review. It may not chain into the 100-case
Validation run. Validation requires a later explicit authorization.

A passing Validation result is only a production candidate. It must not flip a
runtime flag or change prompt/Usage authority. Production activation requires
a separate review of exact credential binding, default flags, rollback,
monitoring, and operational readiness, followed by explicit owner
authorization.

## Failure behavior

The owner selected strict fail-closed degradation. A Luna timeout, transport or
Provider failure, invalid/late output, or protocol drift returns an explicitly
empty v2 Memory set. Normal chat continues without v2 personalization, and the
reader never falls back to recalled, reranked, schema-v10, or otherwise
unjudged candidates. Losing personalization for one turn is preferred over
injecting Memory without current Luna authority.
