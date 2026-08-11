# Double-confirmation schema-v23 Validation preflight

## Frozen identity and Fake result

The consumed schema-v21 result was not rerun. Schema-v23 binds the new v4
production-policy descriptor
`d5d21e747a152bd5b6bfe499c243559f9b45002456438a2ab9165fcad61d1f70`
through independent profile, reader, report, run, capture, cost, approval, and
Vault identities.

PostgreSQL 17 Fake run `memory-regression-20260809t042942z-dd53bb1f`, capture
`79f147cd-437a-4afd-b0ff-b4193ee72ab5`, completed the exact `100/100`
Validation order. Routing was `35` empty-candidate, `10` negative-guard, `55`
Judge-completed, and zero failed. The forced path made `55` primary plus `110`
confirmation decisions: `165` attempts, zero retry, `294993` total input-token
upper bound, `196442` confirmation-only tokens, and `21120` output tokens.

All quality metrics were `1.0` except false injection, which was `0/45`; every
safety/privacy counter was zero. The immutable result is Yellow
`FAKE_PROTOCOL_NON_EVIDENCE`, `passed=false`, `policySelected=false`,
`promotionEligible=false`, and `releaseEligible=false`.

```text
report SHA-256:   2632e98b130ee0e1d898247b60cf0ce093d881eedf1062baa2a4df9538d88e0d
manifest SHA-256: 29e590baa86617e010aa5ce3d699841145d6eea032e50f5ebe94c7c490d7417b
evidence root:    /var/tmp/neo-chat-double-confirmation-validation-v23-fake-20260809T042929Z
```

Both aggregate artifacts are mode `0600`. Every scoped container, network,
and volume was destroyed; no live Provider request occurred.

## Frozen live cost

Maximum three-attempt coverage is `294993 * 3 = 884979`, rounded upward to
`900000`. The final schema-v23 authority binds `900` requests, `900000` input
tokens, `115200` output tokens, and `1015200000000` owner-budget microunits.

```text
path:             /var/tmp/neo-chat-double-confirmation-validation-v23-fake-20260809T042929Z/cost-final-live-validation.json
mode:             0600
raw SHA-256:      62d3ec814d2dcec2f9e79e6d391212d86f557be253e0533250b64d02f995b860
canonical SHA-256: 92563d188a999a108b027b65a543716c3fcf6cb6b3a93f30d997dad5e2cfde41
```

The document passed `DecodeCostBasis` and
`ValidateDoubleConfirmationValidationCostAuthority`.

## Gate and authorization boundary

All backend tests/vet, focused race evidence, generic topology/lifecycle, and
the dedicated Validation Vault lifecycle pass. The product composition root
still installs the historical negative-guard production policy; both Memory
flags are false and the canary is empty.

The owner most recently granted another live v4 double-confirmation
Development authorization. That lane is already consumed and cannot be rerun;
the wording is not a schema-v23 Validation authorization. No live Validation
request was sent. The only next paid authority that can advance the rollout is
one fresh exact `live v23 double-confirmation Validation` authorization.
