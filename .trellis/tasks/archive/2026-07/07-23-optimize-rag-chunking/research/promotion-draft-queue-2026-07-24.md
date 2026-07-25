# Synthetic promotion draft queue — 2026-07-24

## Boundary

This queue is a deterministic aid for later human curation. All questions,
answers, source facts, and identifiers are synthetic. Every case has
`review.state=draft`; the queue declares `promotionEligible=false` and is not a
frozen Golden corpus or activation artifact.

## Source binding

```text
collection UUID:
516c803c-1e59-46f6-8093-d639655bcb2d

source manifest SHA-256:
e014f999f48a3a7cbb0d10fa9f60d8404752bb026ba21ae70ea121f5a6d230df

import receipt SHA-256:
4c727519ef651a1101879c11a8f7de9fed400035351bfdceea49d852a3635645
```

The generator rejects any mismatch in manifest/receipt membership, source
SHA-256, filename, MIME, File UUID, Document UUID, active status, format count,
language count, fact anchors, or draft-only source state.

## Generated artifacts

```text
promotion Golden draft:
promotion-golden-synthetic-draft-2026-07-24.json
SHA-256:
13da3b64f0abb7315f22acc238a11adbdf2e0a8eb164deb94a2508a9f57a056f

review queue with source bindings and expected answers:
promotion-curation-queue-synthetic-draft-2026-07-24.json
SHA-256:
d0fbc33dd621eb10cd7fe7aa3ff68340b912594af97cb316afbe5447f469865b
```

```text
cases:                       500
unique evidence anchors:     500
Development / Validation /
Holdout:                     300 / 100 / 100
Chinese / English:           250 / 250
table-exact cases:            50
human_reviewed cases:          0
promotionEligible:         false
```

Each source contributes ten questions. The split assignment rotates across the
ten fact anchors per document so every source appears in `6/2/2` cases and the
overall query types are not isolated to one split. The review queue binds each
case to its exact `sourceId:factAnchor`, source SHA-256, File UUID, Document
UUID, section, expected label, and expected answer. The closed promotion draft
uses the redacted collection alias
`rag-eval-synthetic-e014f999f48a` rather than the database UUID.

## Replay

```bash
python3 generate-promotion-draft-queue.py \
  /tmp/neo-chat-rag-eval-corpus-20260724/manifest.json \
  live-evaluation-corpus-import-2026-07-24.json \
  promotion-golden-synthetic-draft-2026-07-24.json \
  promotion-curation-queue-synthetic-draft-2026-07-24.json
```

Two independent generator executions produced byte-identical outputs. The Go
decoder also accepted the exact 500-case promotion draft:

```text
schema:                    neo-chat.rag-promotion-freeze-hash.v1
corpus state:              draft
case count:                500
canonical content hash:
bfd19a8c76dd8284c73debc65564e418b562a80a2d911998b00f687d8ab0dc95
promotionEligible:         false
```

The canonical content hash above is diagnostic only. Do not copy it into
`lifecycle.frozenContentSha256`; review identities, timestamps, and freeze
state do not exist yet.

## Curation approval

The operator confirmed the synthetic queue as reviewed for curation use. A
dedicated reviewer identity and immutable input hashes are recorded at:

```text
reviewer UUID:
aaefa8b0-9b7f-4ae2-a7e1-af8aa43ac5d9

approval receipt:
promotion-curation-approval-receipt-2026-07-24.json
receipt SHA-256:
2d0ed6440e9bf9feb37299be12b156789bb42b58a51be3946b6e2f939da55cbd
```

That first receipt changed no case and preserved the seed queue as `draft`.
The reviewer subsequently confirmed that all 500 cases were individually
checked, not merely batch-count or schema approved. The PRD was narrowed so
machine generation alone remains inadmissible while an explicitly reviewed,
hash-bound derivative may transition to `human_reviewed`.

```text
frozen reviewed Golden:
promotion-golden-human-reviewed-frozen-2026-07-24.json
raw SHA-256:
515ec1b598c45aca726735075f3366a64a1dc3a071ca30765ce62f2536e8a1c8

frozen content SHA-256:
17eee60e93dc32733a8b871ccc7176d8a5f02f283eaebc37113fcb40ec43ce8c

human-review receipt:
promotion-human-review-receipt-2026-07-24.json
receipt SHA-256:
39f076402dbbec233b4214b76a442548e13f85c62dd5f98f1bee8992253087f8

reviewed cases:        500 / 500
reviewer UUID:         aaefa8b0-9b7f-4ae2-a7e1-af8aa43ac5d9
reviewed/frozen at:    2026-07-24T02:29:04Z
Holdout Run UUID:      ba1749c9-922e-4ab7-b6c0-37603e582617
promotion gate run:    false
promotionEligible:     false
activation executed:   false
```

The unchanged seed queue remains a replay/rollback anchor. The reviewed Golden
is now admissible as an evaluator input, but it is not promotion-eligible until
independent Active/Candidate observations pass every gate. Holdout has not been
executed.
