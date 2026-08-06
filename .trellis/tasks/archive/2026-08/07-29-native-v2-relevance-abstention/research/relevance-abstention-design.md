# Native v2 relevance-abstention design research

## Question

How should Neo Chat prevent the native v2 hybrid Memory reader from sending or
injecting irrelevant Memory while retaining its measured semantic recall?

## Evidence from the first live run

- `native_v2_hybrid` reached 100% Candidate Recall@20, Final Recall@5,
  current-fact accuracy, nDCG@5, and MRR@5 across 500 cases.
- It returned Memory for all 50 `unrelated_negative` cases. Those 50 cases
  account for every candidate false injection and every unauthorized Provider
  egress event in the report.
- Current code validates and sorts `relevance_score`, then discards it. The
  final selector admits every ordered candidate that fits the Top-5 and token
  limits.
- The stored evidence contains IDs and aggregate metrics, not raw rerank or
  vector scores. A safe threshold cannot be reconstructed from that run.

## Model and Provider facts

- SiliconFlow documents `relevance_score` as a value in `[0,1]` where higher
  means more relevant. The API exposes ranking and scores, but specifies no
  universal relevance threshold:
  <https://docs.siliconflow.cn/cn/api-reference/rerank/create-rerank.md>.
- The BAAI model card says `bge-reranker-v2-m3` produces an unnormalized score
  that may be mapped to `[0,1]` with a sigmoid. That makes the output ordered
  and bounded, not a calibrated probability for Neo Chat's Memory corpus:
  <https://huggingface.co/BAAI/bge-reranker-v2-m3>.
- Existing Neo Chat RAG evidence already warns that useful and unused
  reranker scores can overlap. A guessed global threshold is therefore not
  acceptable without corpus-specific calibration.

## Two distinct gates are required

### 1. Pre-rerank Provider-egress admission

A post-rerank threshold cannot undo Memory plaintext already sent to the
hosted reranker. Before rerank, the production SQL/Go path must expose only a
transient, finite local retrieval signal (for example the best authorized BGE
cosine signal plus lane evidence) and abstain before document egress when that
signal is below a frozen admission policy.

The signal may be returned transiently with the already-authorized candidate
set, but must never be written to observations, logs, chat metadata, or report
artifacts. Missing, non-finite, stale, or uncalibrated signals fail closed.

### 2. Post-rerank final-injection admission

The production rerank result must retain each finite score in request-local
memory. Final selection first removes rows below a frozen relevance policy,
then applies the existing Top-5 and 600/900 token budgets. If no row passes,
the result is an explicit `no_memory` abstention.

This score also remains transient. Durable evidence records only the versioned
policy identity, bounded status/counts, and authorized IDs already permitted
by the existing observation contract.

## Approaches considered

### A. Split-safe, fixed two-stage policy (recommended)

1. Add a calibration-only live lane that evaluates a precommitted, bounded
   threshold grid in memory.
2. Use only the 300-case development split to choose the lowest pre-rerank
   threshold with zero unauthorized Memory egress and the lowest post-rerank
   threshold that satisfies the false-injection gate while maximizing recall.
3. Persist only aggregate counts/metrics by threshold and slice; never raw
   scores, case IDs, queries, Memory content, or credentials.
4. Freeze the selected values and policy/version in code and in the capture
   configuration hash.
5. Run the unchanged 100-case validation split once with the frozen policy.
   The visible regression `holdout` may be reported but is not formal Holdout
   authority and cannot be used for tuning or promotion.

Benefits: reproducible, privacy-preserving, rollback-friendly, and directly
bound to the measured model/corpus. Cost: requires a calibration run followed
by an independently authorized validation run.

### B. Guess a scalar such as `0.5`

Rejected. The API range does not make the score a calibrated probability, and
the first run retained no distribution from which to justify a guess.

### C. Dynamic top-score/margin policy immediately

Deferred. A margin or query-class policy may outperform a fixed scalar if the
development curve has no feasible operating point, but it adds more parameters
and overfitting risk. It should be introduced only after approach A proves that
a fixed policy cannot simultaneously satisfy recall and safety.

## Development result and diagnostic follow-up

The authorized 300-case Development run on 2026-07-29 evaluated all `20,301`
precommitted scalar pairs. The Provider cost ratio was `0.033084` and passed,
but `feasiblePairCount=0`; no policy was selected. This activates the deferred
query-class/intent/margin branch but does not by itself select one.

The v1 report retained only a feasible frontier. With no feasible point, its
frontier metrics are zero values and cannot show whether admission similarity,
maximum rerank score, or top-two rerank margin separates relevant cases from
unrelated negatives. Raw traces were destroyed after aggregate publication as
required, so the missing distributions cannot be reconstructed.

The next Development run must use calibration schema v2 and publish only:

- aggregate failure counts over the fixed 20,301 pairs;
- the safety-first and recall-first attempted pairs, even when neither passes;
- cumulative relevant/unrelated-negative passing counts over the fixed
  admission, maximum-rerank, and top-two-margin threshold grids;
- eligible/missing counts, with single-candidate rerank rows marked missing
  from the margin curve rather than assigned a fabricated margin.

The diagnostics version is configuration-hashed before Provider work. It
still stores no case ID, query, Memory content, raw per-case score, or
credential. Validation remains forbidden until Development evidence yields a
passing policy that is frozen in code.

### Schema-v2 diagnostic result

The fresh schema-v2 Development run again produced `20,301/0` feasible scalar
pairs with cost ratio `0.033084` passing. Its aggregate evidence rules out the
remaining score-only branches:

- local admission: the lowest threshold with zero unrelated Memory egress is
  `0.85`, retaining `20/165` relevant cases (`12.12%`);
- maximum rerank score: unrelated cases survive through `0.99`; `1.00` is the
  first zero-unrelated threshold and retains no relevant case;
- top-two rerank margin: all `30` unrelated cases and `135/165` relevant cases
  have only one candidate, so missing-margin fail-closed can retain at most
  `30/165` relevant cases (`18.18%`).

Therefore neither a scalar, maximum score, nor candidate margin can meet zero
unauthorized Memory-document egress and the unchanged recall gates.

### D. Query-only bilingual intent margin (completed, infeasible)

Before Memory-document rerank, compare the already-secret-redacted query with
two fixed bilingual documents: one describes requests where stored personal
Memory may help; the other describes requests answerable without personal
Memory. The fixed SiliconFlow BGE reranker returns request-local positive and
negative scores, and the policy calibrates `positive - negative` over a
precommitted `[-1.00,1.00]` step-`0.01` Development grid.

This call sends no Memory plaintext. Query egress is not a new trust boundary
because the same redacted query already reaches the fixed BGE embedding model.
The anchor version and exact SHA-256 are configuration-hashed; anchor text
changes require a new version. Missing/invalid/late/drifted classification or
a margin below threshold fails closed before local admission and candidate
rerank. A 201-threshold aggregate report retains only cumulative counts,
failure totals, best attempts, and an optional passing selection.

The authorized schema-v3 Development run completed all `201` thresholds. Its
`providerCostRatio=0.056284` passed, but
`intentFeasibleThresholdCount=0`. At threshold `0.04`, all unrelated cases are
rejected and both false injection and unauthorized Memory-document egress are
zero, but only `31/165` relevant current-fact cases remain (`18.79%`). At the
recall-first threshold `-0.09`, recall/current-fact accuracy is `1.0`, but `26`
false injections and `26` unauthorized egress events remain.

The query-only margin therefore has the same structural conflict as the local
similarity signals: it cannot know whether a particular candidate Memory is
useful without seeing that candidate. Adding benchmark-derived lexical rules
or iterating more anchors would tune to Development without establishing a
general separating signal. The policy remains unavailable and Validation does
not run.

### E. Candidate-aware private decision boundary (next research class)

The next policy must compare the redacted query with the current authorized
candidate content before deciding whether any Memory may leave the private
server boundary. Because sending the candidate to a hosted judge is already
the egress event being authorized, that decision cannot be delegated to the
same hosted boundary without circular authority.

The defensible next class is a local/private candidate-aware judge (for
example a self-hosted multilingual cross-encoder or strict structured local
LLM judge) executed after SQL reauthorization and before hosted Provider
egress. It must remain default-off, use a version/hash-bound model and prompt,
retain no plaintext or raw scores, and fail closed on missing model, timeout,
invalid output, or authority drift. Development must precommit its decision
schema and objective; Validation remains unavailable until one frozen local
policy passes every unchanged gate.

This is a separate model/profile and deployment decision, not a threshold
continuation. It must be evaluated against hardware, latency, model licensing,
image size, offline reproducibility, and the existing two-second request-local
cutoff before implementation.

### F. Owner-authorized cloud candidate judge (selected)

The owner subsequently confirmed that this single-user Server-mode deployment
may send ordinary personal Memory to the configured cloud Provider. This
changes the trust-boundary authority, not the relevance goal: an unrelated
candidate may cross the Provider boundary under the exact owner opt-in, but it
must still never enter the answer prompt.

The selected next experiment therefore keeps SQL current-user/scope/revision/
hash/epoch/generation reauthorization and deterministic secret redaction, then
runs the fixed BGE reranker and a fixed cloud LLM candidate judge concurrently.
The judge receives only the redacted query plus candidate bodies labelled by
request-local ordinals. It returns exact strict JSON containing zero to five
unique ordinals; Memory IDs, raw scores, prompts, and content remain absent
from durable evidence. The final selector intersects those ordinals with valid
BGE order and existing token limits.

Evaluator semantics must be policy-aware rather than silently weakened. Under
the exact versioned owner authorization, `irrelevant` candidate processing is
authorized, while cross-user, out-of-scope, deleted, secret, superseded,
Sensitive-disabled, and untrusted-source egress remains a hard failure. False
injection remains unchanged and independently gated. Missing authorization,
judge/model/prompt drift, malformed output, timeout, or Provider failure is
`no_memory`.

## Failure and fallback policy

- The v1 prompt/Usage reader remains unchanged during shadow evaluation.
- For the candidate surface, missing/invalid pre-admission evidence, rerank
  failure, redaction, timeout, or score-policy drift should produce no hybrid
  final/injection rather than unscored RRF output.
- This is precision-first: losing optional Memory on an uncertain turn is
  safer than injecting a wrong personal fact. The normal chat answer still
  proceeds without hybrid Memory.

## Versioning and rollback

- The existing embedding/RRF tuple may remain a retrieval profile, but the
  selector/calibration policy must receive a new immutable reader/policy
  version and be included in `configurationSha256`.
- All flags stay default-off. The implementation adds no promotion authority.
- Rollback restores the previous selector version and leaves v1 as the only
  prompt/Usage authority.

## Implementation seams

- `backend/internal/usermemory/hybrid_shadow.go`: transient scored candidates,
  pre-rerank admission, rerank abstention, and fail-closed final selection.
- `backend/internal/usermemory/types.go` and the hybrid repository: bounded
  transient admission evidence without durable raw scores.
- `backend/internal/memorycapture`: aggregate-only calibration, versioned
  profile hash, and split-safe validation output.
- `backend/internal/memoryeval`: reuse existing metrics; do not weaken safety
  or promotion rules.
- `scripts/run-memory-regression.sh` and docs/tracking: exact calibration and
  validation operator flow, live status, teardown, and evidence handling.
