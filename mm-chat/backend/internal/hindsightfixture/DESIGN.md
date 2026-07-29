# Hindsight fixture adapter design

## Goals

- Measure the pinned Hindsight implementation on Neo Chat's synthetic Memory
  semantics without exposing Live Memory, chat, Provider credentials, or the
  primary database.
- Separate extraction behavior from retrieval behavior with two fixed profiles.
- Preserve replayable, content-free evidence while deleting all Hindsight
  runtime state after each comparison.
- Fail closed on authority drift, cross-bank results, malformed responses,
  timeout, cancellation, or cleanup failure.

## Non-goals

- No production `MemoryEngine`, API route, frontend control, migration,
  canonical write, prompt injection, Usage/Activity record, or reader flag.
- No Live Provider and no 30-day real-data trial.
- No generated Hindsight SDK and no vendor source in Neo Chat.
- A passing draft report is not promotion evidence and cannot retain an
  instance or change any Native Memory authority.

## Flow

```text
checked-in synthetic manifest --strict decode + canonical hash--+
                                                               |
checked-in Golden corpus --strict decode + fixture binding-----+
                                                               v
ephemeral API key --HMAC--> opaque bank/document IDs --> Runner
                                                       | configure
                      rejected rows stop here -------->| retain
                                                       | delete tombstone docs
                                                       | recall by fixed scope tags
                                                       v
                                       content-free report to stdout
                                                       |
                             scoped Compose down --volumes + zero-object proof
```

`end_to_end` sends `rawEventContent` through Hindsight's local mock extraction.
`retrieval_only` sends `canonicalContent` with extraction mode `chunks`. Both
use local embedding/reranking from the pinned full image. The internal Docker
network has no external route. Explicit Hugging Face/Transformers offline and
LiteLLM local-cost-map variables prevent preloaded models from attempting
metadata refreshes; the private network provides an independent zero-egress
boundary. The mock LLM provider/model are static server environment fields;
the bank PATCH changes only hierarchical extraction/audit fields.

## Authority decisions

| Decision | Reason |
| --- | --- |
| Standard-library `net/http` adapter | Keeps the audited REST surface small and avoids generated SDK drift. |
| HMAC-derived bank IDs | Binds bank identity to ephemeral key, manifest hash, mode, fixture alias, and user alias without exposing Neo Chat user IDs. |
| Logical Memory ID in Hindsight metadata | Maps results without returning Hindsight text, score, trace, or database identifiers. |
| One bank per fixture alias | Makes cross-user/case isolation explicit and independently deletable. |
| Global `tags=[]/exact`; scoped `tags_match=any` | Global recalls cannot see tagged facts; Project/Conversation recalls may include untagged Global facts. Conversation uses only its conversation tag. |
| API bank delete plus whole-project destruction | Bank deletion narrows data, but only database/volume/container/network/key destruction covers audit, traces, queues, caches, files, and logs. |
| stdout-only runner report | The read-only container receives no output mount; the host wrapper validates and exclusively publishes the report. |

## Threat model and controls

| Threat | Control |
| --- | --- |
| A fixture smuggles an endpoint, bank ID, DB URL, key, or unreviewed field | Strict unknown-field rejection; the schema contains none of those fields. |
| Duplicate JSON shadows a policy field | Recursive duplicate-key rejection before typed decoding. |
| Real or sensitive data is presented as a benchmark | Explicit synthetic-only/no-real/no-sensitive/promotion-ineligible declarations, fixed checked-in mounts, and no Live state mounts. |
| Cross-bank response is accepted | Each logical Memory ID has one manifest owner; owner mismatch fails the case. |
| Secret/untrusted sentinel reaches Hindsight | Those states are classified before HTTP and never retained. |
| Raw upstream failure leaks content or credentials | HTTP bodies are discarded on non-2xx; only a bounded fixed error code reaches the report or stderr. |
| Malformed or oversized API response consumes memory or changes meaning | 4 MiB response limit plus duplicate/unknown/trailing strict JSON decoding. |
| Canceled run leaves a bank or trace | Runner best-effort bank cleanup uses a bounded background context; the shell trap then destroys the exact random Compose project and credential directory. |
| Cleanup targets Native runtime | Dedicated Compose file/project/network/volume; no main network or protected runtime mount; no broad Docker prune and no main-project `down -v`. |

## Known limitations

- The checked-in corpus has ten draft cases, not the required human-reviewed
  500-case frozen corpus. Reports are smoke/comparison evidence only.
- The pinned local embedding and reranker models are primarily English. Chinese
  and mixed-language performance must be read directly from the fixture result.
- Hindsight's mock extraction verifies pipeline mechanics, not hosted-LLM
  extraction quality.
- The pinned full image contains the local weights but still requires explicit
  offline metadata variables; without them startup attempts remote model/cost
  metadata resolution and fails on the private network.
- The report contains logical opaque Memory IDs, but no text or score, so a
  failed case requires an authorized synthetic-only rerun or offline test to
  diagnose ranking internals.

## Change history

- 2026-07-29: initial PR13 fixture-only adapter, dual profile, isolated Compose
  topology, sanitized report, and scoped teardown.
