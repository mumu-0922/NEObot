# Schema-v12 false-injection retrospective

## Evidence boundary

This analysis uses only the retained aggregate schema-v12 report and the
checked-in synthetic Development generator/corpus. It makes no Provider call,
uses no credential, inspects no raw Provider response, and does not reinterpret
the immutable report. The retained run is
`memory-regression-20260731t080147z-8649c8ae`; report SHA-256 is
`126536772d71a5815f1cb6029deb568d0655c8780924ac0428951807975c8011`.

## Decisive distribution

All 195 candidate-bearing Development cases completed judging. Luna returned
an empty ordinal list for only six cases:

```text
relevant cases with Memory     = 160 / 165
relevant cases abstained       =   5 / 165
unrelated-negative with Memory =  29 / 30
unrelated-negative abstained   =   1 / 30
```

The report-level false-injection rate is `29 / 300 = 0.096667` because the
evaluator uses every Development case as its denominator. The conditional
false-positive rate inside the only negative candidate-bearing slice is
`29 / 30 = 0.966667`. This is not a marginal threshold miss: the fixed Luna
judge almost always selects a candidate whenever one is presented.

The failure crosses language templates. The aggregate slice intersections
prove false injection in all 22 Chinese-paraphrase cases, all six mixed-language
cases, and one of the two remaining English cases. It is therefore not a
Chinese-only or translation-only defect.

`deletion`, `scope_isolation`, `secret_rejection`, and `untrusted_source`
false-injection failures are overlapping labels on four of those same
unrelated-negative cases, not separate authority leaks. Their actual safety
counters are all zero. The seed path skips secret/untrusted records, makes
deleted records non-current, and queries by the current user/scope. Only the
ordinary current-authorized `StateIrrelevant` body can reach the judge in this
negative slice.

## Contract mismatch

The current synthetic negative pair is self-referential:

```text
query     = Should an unrelated note influence <subject> for <entity/scope>?
candidate = An unrelated weather note near <scope> that has no bearing on
            <entity>'s <subject>.
expected  = no Memory
```

The Chinese and mixed-language templates encode the same relationship. The
query explicitly asks about the candidate's irrelevance, while the candidate
explicitly states the answer. Under the frozen prompt instruction to select
stored information that is "directly useful for answering the query", choosing
that candidate is a plausible literal result. Under the evaluator contract,
any choice is false injection because `unrelated_negative` always requires
`expectedNoMemory=true`.

The benchmark is trying to measure epistemic necessity (whether the answer
*depends on a user-specific saved fact*), while prompt v1 asks only for direct
answer usefulness. Those are different predicates. Model hopping cannot repair
that semantic mismatch reliably, and another live run under the unchanged
prompt/corpus would only spend quota on the same ambiguity.

## Recommendation

Do not retune schema v12, weaken the `0.02` gate, or automatically run another
model. First choose and version one contract repair:

1. **Recommended: new regression-corpus version.** Replace self-referential
   negative queries with genuine user tasks where a same-entity/same-scope
   Memory is topically similar but not useful. Do not label the candidate body
   itself as "unrelated" or "has no bearing". Keep the current corpus and every
   historical report immutable, regenerate new hashes/identities, and rerun
   baselines before judging a successor.
2. **Alternative: prompt-v2 epistemic-necessity contract.** Preserve the
   adversarial corpus but require selection only when a user-specific answer
   depends on a candidate; questions answerable from their own wording or
   general policy must abstain. This is a new prompt/profile hypothesis, not a
   schema-v12 retry, and it needs broad non-benchmark unit fixtures plus fresh
   live authorization.

Before either implementation, obtain an explicit owner decision because the
choice changes what the benchmark means. Validation, production, and promotion
remain blocked in both paths.

## Owner decision and offline repair outcome

The owner selected option 1: preserve every v2 byte/report and add an
independent `memory-regression-zh-mixed-v3` generator. The repaired
`unrelated_negative` query asks for a neutral agenda heading. Its
same-entity/same-scope candidate records a lobby weather-board observation;
the candidate cannot supply the requested heading and does not label itself as
irrelevant. No prompt, model, threshold, active reader, or evaluator criterion
changed.

The offline implementation uses generator
`neo-chat.memory-benchmark-regression-generator.v2`, seed `2026073101`, and
auditor `deterministic-semantic-audit.v2`. Exact profile dispatch prevents
unknown tuples and v1/v2-generator artifact mixing. The v3 semantic audit
requires the agenda/weather markers and rejects `unrelated`, `无关`,
`no bearing`, and `没有关系` from the query/candidate pair.

The generated content-free v3 bindings are:

```text
fixture content SHA-256 = 49cae3861be4eade46c8d042ab4d4d4c3d779ba95a6c6b29cf77de5374e7c71a
corpus content SHA-256  = cfa666c0771f6375058a23d117613b57f32beb22ae460eb16db50ddb325897df
audit content SHA-256   = 2803504064da8625e78c60dcad5043adfc7cf1cde2079daa3f60fd060ccd68f9
manifest raw SHA-256    = dd87967b2d2e2c48c6c21c2b17b2b0d0b2cecd6c3998b23dbee351e92056bf09
```

Disposable generation and byte replay passed with a mode-`0700` root and four
mode-`0600` artifacts. `/tmp` was unavailable because this machine already has
a `/tmp/.git` repository marker and the authoring boundary correctly rejects
output inside another Git repository, so the disposable proof used
`/var/tmp/neo-chat-memory-v3-kNSoNn/v3-regression`. Focused race tests, all
backend Go tests, and `go vet ./...` passed. No Docker command, database,
Provider request, credential, Validation run, production mutation, or
promotion action occurred.
