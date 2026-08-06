# Memory Judge buffered-transport design research

## Question

How should the fixed Luna candidate Judge remove the schema-v16 streaming
transport failure surface without changing relevance policy, prompt, guard,
scoring, corpus, or production Memory authority?

## Runtime evidence

- The retained schema-v16 live report is
  `/var/tmp/neo-chat-negative-guard-development-20260806T064153Z/live-runs/20260806T064355Z-65407a6a/fixed-memory-judge-negative-guard-development.json`.
- It completed all 300 cases with 185 Judge attempts and 20 retries.
- Attempt failures were eight `PROVIDER_STREAM_READ_FAILED`, thirteen
  `PROVIDER_TRANSPORT_FAILED`, and two `PROVIDER_UPSTREAM_FAILED`.
- Three cases ended terminally as `PROVIDER_TRANSPORT_FAILED`; five completed
  Judge decisions selected no Memory. The report remained failed and
  non-promotional.
- The v16 run is immutable evidence and must not be rerun. It is sufficient as
  the representative streaming baseline for a new transport-only successor.

## Current code path

- `internal/memoryjudge.ChatAdapter` always calls `chat.Provider.StreamChat`.
- `internal/chat.OpenAICompatibleProvider.StreamToolRound` always sends
  `stream:true` and consumes SSE with `bufio.Scanner`.
- The same Provider already owns a bounded non-streaming JSON request path in
  `OpenAICompatibleProvider.planTools`, so JSON completion transport can remain
  inside the Provider package instead of duplicating the OpenAI-compatible wire
  format in `memoryjudge`.
- Schema-v16 already provides serial requests, two Judge-only transient
  retries, fixed five/ten-second fallback waits, typed aggregate failures,
  one-second inter-case cooldown, and exact cost reconciliation.

## Invariants for a transport-only successor

The following must remain byte/hash/equality-bound to schema v16:

- fixed `SERVER_DEFAULT` / `openai_compatible` / base-URL hash /
  `gpt-5.6-luna` authority;
- fixed BGE-M3 embedding/rerank tuple;
- negative-policy guard version/SHA and policy descriptor SHA;
- candidate-Judge prompt version/SHA, temperature, no-thinking control,
  128-token maximum, and strict output decoder;
- 300-case Development split, criteria v3, case order, Provider-egress rules,
  retry count/timing, and cost ceilings;
- aggregate-only artifacts, cleanup, default-off runtime flags, and zero
  production Memory mutation.

Only these surfaces should differ:

- request `stream:false` and `Accept: application/json`;
- bounded JSON-envelope/body parsing instead of SSE parsing;
- separately versioned adapter, execution sequence, capture/profile/reader/
  report/cost identities.

The existing failure taxonomy can remain v1. HTTP status classification is
unchanged; a buffered body read interruption maps to the existing retryable
`PROVIDER_TRANSPORT_FAILED`, while malformed/oversized envelopes remain
deterministic `PROVIDER_RESPONSE_INVALID` failures.

## Approaches

### Approach A: versioned buffered full-Development lane (recommended)

Add a small optional buffered completion interface to `internal/chat`,
implement it in `OpenAICompatibleProvider`, and add a strict buffered adapter
to `internal/memoryjudge`. Bind it to a new schema-v17 300-case Development
lane while using the retained schema-v16 result as the streaming baseline.

Pros:

- changes the exact suspected transport variable under representative load;
- avoids a prohibited v16 rerun and avoids a synthetic microbenchmark surface;
- preserves Provider-specific wire ownership in `internal/chat`;
- produces directly comparable aggregate quality and stability evidence.

Cons:

- the full run is consumed even if the new transport still fails;
- quality may remain below criterion after transport failures reach zero.

### Approach B: increase streaming retries

Keep SSE and increase retry count/backoff under a new identity.

Pros: smallest implementation diff.

Cons: does not remove the observed SSE/read surface, increases duplicate
requests and run time, and confounds transport reliability with retry budget.

### Approach C: separate dual-arm microprobe

Create a new synthetic probe command that repeats streaming and buffered calls
before changing the full lane.

Pros: quick wire-level comparison.

Cons: payloads and cadence are less representative, creates another artifact
and credential lifecycle, and repeats the already-retained streaming arm.

## Recommendation

Use Approach A. Implement schema-v17 as a buffered transport-only successor,
run mandatory Fake lifecycle, then one complete authorized live Development
run. Stop and retain the result whether it passes or fails. If transport is
clean but positive quality still fails, address Judge abstention/prompt behavior
only in a later separately versioned task.

## Relevant specifications and files

- `.trellis/spec/backend/memory-v2-benchmark.md`
- `.trellis/spec/backend/memory-v2-hybrid-shadow.md`
- `mm-chat/backend/internal/chat/provider_openai_compatible.go`
- `mm-chat/backend/internal/memoryjudge/chat_adapter.go`
- `mm-chat/backend/internal/memorycapture/accuracy_first_providers.go`
- `mm-chat/backend/internal/memorycapture/negative_policy_guard_memory_judge_development.go`
- `mm-chat/backend/cmd/memory-regression-capture/main.go`
- `mm-chat/scripts/run-memory-regression.sh`
- `mm-chat/scripts/run-memory-negative-guard-development-from-vault.sh`

