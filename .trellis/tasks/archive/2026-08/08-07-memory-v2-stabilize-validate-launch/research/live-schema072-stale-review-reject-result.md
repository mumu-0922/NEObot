# Live schema-072 stale Review rejection result

## Root cause

The sole pending Review referenced target Memory
`b6c66389-54aa-4fce-a91f-39ba4ee76352` at revision `1`. The target later
advanced to revision `2`. The migration-`060`
`memory_governance_decide_review(...)` capability checked epoch, scope, and
target currentness before every decision, so the UI's `reject` request failed
with stale-Review behavior even though rejection never consumes or mutates the
target. The candidate was stranded.

Migration `072_memory_review_stale_reject` keeps current-user ownership,
pending status, 30-day expiry, decision shape, and replay hash ahead of every
branch. Only `reject` skips epoch, scope-generation, and target lifecycle/
revision validation. It may wipe the candidate, complete the link-only
Activity, and append the existing plaintext-free decision audit.
`keep_current`, `accept_new`, `edit_merge`, and `keep_both` keep all original
current-authority fences. No HTTP surface, function signature, role, grant, or
table changed.

## Offline verification

A disposable PostgreSQL 17 instance passed the exact
`071 -> 072 -> 071 -> 072` lifecycle. The test proved:

- stale-target `reject` fails at `071` and after migration `072` is down;
- stale-target `reject` succeeds after `072` up/re-up through
  `go_api_runtime`;
- the target content hash and revision remain unchanged;
- candidate plaintext is wiped and exactly one replay-safe decision remains;
- same-hash replay is idempotent and conflicting replay fails closed; and
- `keep_current`, `accept_new`, merge, and keep-both retain stale authority
  denial.

Focused migration race tests, every Memory PostgreSQL migration test, all
Backend tests, and `go vet ./...` passed. The full standalone verifier also
passed Frontend `964/964`, all Backend gates, and RAG
`1906 passed / 7 skipped`.

## Live execution

The first live harness attempted to verify nonexistent `up_checksum` and
`down_checksum` migration columns. Its prepared rollback immediately downed
`072` and restored the prior runtime. Schema, Review, Memory, service image,
flags, and unrelated container IDs were unchanged. This was a verifier defect,
not a migration or product failure.

A new evidence root and backup were used for the corrected retry. The frozen
candidate applied migration `072`, recreated only `backend` and
`memory-worker`, and then submitted the exact pending `reject` through the
authenticated API. The result was:

```text
schema=72
reject HTTP=200
pending_reviews=0
decision=reject
status=rejected
resultCode=USER_REJECTED
candidate plaintext=0
```

Canonical target state remained byte-authoritative:

```text
target=b6c66389-54aa-4fce-a91f-39ba4ee76352
revision=2
content_hash=82f266e334944342ee7486920e3bf4477cc0c149d436f483e1ca49ce3a87388e
lifecycle=active
```

The deployed runtime is:

```text
image=mm-chat/backend:memory-v2-schema072-stale-review-reject-candidate-20260810t063127z
image_id=sha256:6b1405eba79001fd444c48303cef11d6c0e0444b654bdbec3532e1a5ed486889
backend=healthy
memory-worker=healthy
```

Memory remained ready with two current L1 rows. L2 Scene and L3 Persona each
retained one current shadow artifact and zero promotion events. Their shadow
generation flags stayed true and both active reader flags stayed false.
Unrelated container IDs did not change and startup logs contained no
`ERROR`, `FATAL`, or panic.

## Evidence and rollback

Successful private evidence is retained at mode `0700/0600` under:

```text
/var/tmp/neo-chat-schema072-stale-review-reject-live-20260810T063746Z
```

The verified PostgreSQL custom-format backup is retained at mode `0600`:

```text
mm-chat/backup/postgres/postgres-schema072-stale-review-reject-20260810T063746Z.dump
```

Migration `072` has been consumed and must not be applied again. A database
down would restore the old rejection defect and is no longer the operational
rollback. The safe runtime response to an unrelated regression remains the
existing behavior rollback for the owning Memory flags/readers on the pinned
schema-072 image.
