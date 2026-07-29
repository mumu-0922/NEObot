# Memory benchmark authoring design

## Goals

- Produce a reproducible candidate pool with enough stratified surplus for a
  human to reject weak cases while retaining the formal 500-case gates.
- Preserve authentic case-by-case human review across multi-day sessions.
- Make content, fixture, decision, freeze, and Holdout drift independently
  detectable.
- Keep formal corpus bytes outside Git and all authoring behavior outside the
  production Next.js/API/Compose/runtime boundary.
- Fail closed before a Holdout retry can become a tuning signal.

## Non-goals

- Generate review authority, reviewer identities, or accepted decisions.
- Read or de-identify live chat/Memory data.
- Produce Native reader observations or change a reader pointer.
- Prevent a malicious machine owner from copying files with direct filesystem
  access. The read-once guarantee belongs to the supported toolchain and
  private-file operational boundary, not to DRM.

## Architecture

```text
versioned templates + fixed profile/seed
                |
                v
      deterministic Generate
                |
                +--> fixture manifest --hash--+
                |                             |
                +--> 650-case Golden --------+--> candidate manifest
                                               |
                                               v
loopback browser --> explicit review event --> immutable hash chain
                         |                          |
                         +--> edit invalidation ----+
                                                    v
                                   replayed current review state
                                                    |
                             exact 500 + all gates + Holdout UUID
                                                    v
                                    immutable frozen artifacts
                                                    |
                              exclusive consumed marker (ordinal=1)
                                                    |
                                                    v
                                      bounded Holdout run bundle
```

`memoryauthor` owns candidate fixtures and review/freeze mechanics.
`memoryeval` remains the single source of truth for Golden schema validation,
canonical freeze hashing, formal admission, scoring, and report semantics.

## Core decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Fixed 650-case pool | Provides 30% screening surplus over 500. | Rejected cases are never silently replaced or auto-selected. |
| `390/130/130`, `455/130/65`, slice `39/13/13` minimums | Scales the formal proportions and preserves a deterministic feasibility witness. | Human rejection can still create a quota deficit; freeze then refuses. |
| Generic fixture v1 schema | Avoids coupling Native evidence to the deleted Hindsight fixture. | Fixture and Golden validators must verify logical-ID and authority semantics together. |
| One immutable event file per review action | A completed event is atomic and process death cannot tear an append stream. | More files are created, but 650–1,500 small events are bounded and auditable. |
| Hash chain plus expected sequence/content hash | Detects tamper, forks, stale tabs, and lost-response retries. | Any ambiguous published event state blocks progress. |
| Replaceable checkpoint is non-authoritative | A crash after event publication must not lose review work. | A valid stale checkpoint is tolerated and rebuilt on the next mutation. |
| Edit and decision are separate actions | An edited case has not been reviewed merely because the edit was saved. | Every edit returns the case to `pending`. |
| Loopback standard-library server | Keeps formal content out of production frontend/API/Compose. | The server exists only while the operator command is running. |
| Holdout marker before output publication | A failed first run cannot be retried and tuned. | A crash after marker creation taints the corpus and requires a new version. |
| Existing evaluator admission reused | Prevents authoring and evaluation gates from drifting. | Freeze fails whenever evaluator v1 rejects the assembled Golden. |

## Deterministic generation

The generator fixes its schema, version, profile, seed, template ordering,
language allocation, split ordering, ID derivation, timestamps, and canonical
JSON encoding. It makes no model, Provider, database, clock, environment, or
network call.

Each case owns one fixture alias. Fixture state supplies executable semantic
evidence for active, superseded, irrelevant, untrusted, synthetic-secret,
cross-user/out-of-scope, and deleted exclusions. Diagnostics reject exact and
normalized duplicate queries, missing references, label-only coverage, and a
pool without a concrete exact 500-case split/language/slice witness.

The feasibility witness is diagnostic only. It is hashed into the candidate
manifest but never accepted automatically and never becomes human authority.

## Review state machine

```text
base candidate
   |
   +-- accept --> accepted
   +-- reject --> rejected
   +-- edit ----> pending(new content hash)

accepted/rejected -- edit --> pending(new content hash)
accepted <-----------------> rejected   (new explicit decision event)
```

An event binds sequence, previous event hash, action, immutable case ID,
before/after content hashes, fixture hash, explicit reviewer UUID, and the
server timestamp of the individual action. Edit carries the complete new
case/fixture snapshot so replay needs no mutable side file.

The writer uses a process-scoped nonblocking file lock. Event bytes are synced
to a private staging file and linked exclusively into the event directory.
The event directory is replayed in filename/sequence order; unknown entries,
loose permissions, symlinks, malformed JSON, hash mismatch, gap, fork, unknown
case, stale content, or invalid edited semantics all stop replay.

Single-case validation is not enough because an otherwise valid edit can
duplicate another query or logical Memory ID. Before publishing an edit and
again after full ledger replay, the package materializes all 650 current
snapshots and validates global IDs, normalized queries, fixture/Golden
bindings, slice semantics, and counts. Content-free status counts come from
that materialized state; the candidate-manifest hash remains the immutable
generator-profile binding.

## Review HTTP boundary

The listener is fixed to `127.0.0.1` on an operating-system-selected port.
Callers cannot configure a LAN address. The initial page is content-free and
receives the random bootstrap token only through the URL fragment. JavaScript
exchanges it once for an `HttpOnly`, `SameSite=Strict` cookie and a separate
CSRF token.

Every request verifies the exact loopback Host and client address. Mutations
also verify exact Origin, session, CSRF, method, content type, strict bounded
JSON, current ledger sequence, and current content hash. Responses use
`no-store`, a nonce-based CSP, `frame-ancestors 'none'`, no CORS permission,
and restrictive browser policy headers. UI rendering uses `textContent` or
textarea values; authored JSON is never assigned to HTML.

## Freeze and Holdout

Freeze requires all 650 cases to have a current decision, exactly 500 accepted
and 150 rejected, exact split/language counts, all semantic/slice gates, review
times no later than freeze, and a new precommitted Holdout UUID. Accepted
fixture/Golden bytes are published privately and exclusively, with the freeze
manifest written last. Any incomplete frozen directory blocks further review
or in-place retry.

After freeze, the review server refuses to start. The supported Holdout command
preflights its private, absent output path; assembles and validates the bounded
100-case bundle in memory; then exclusively publishes `consumed.json` before
publishing the bundle. The marker binds UUID, ordinal one, Golden/fixture
hashes, timestamp, and output path. Marker existence permanently rejects a
second attempt even if the first publication or downstream reader failed.

## Threat model and controls

| Threat | Control |
| --- | --- |
| Real data or credentials enter the corpus | No live input surface; fixed synthetic generator and explicit policy fields. Secret cases use non-secret sentinels. |
| Duplicate/unknown JSON changes authority | Recursive duplicate-key, unknown-field, trailing-value, size, enum, and bound checks. |
| Corpus bytes enter Git | CLI allows only the gitignored Memory benchmark data root inside this repository and rejects any other Git repository. Standalone copy excludes `data/`. |
| Path traversal or symlink redirects writes | Absolute containment, forbidden component, existing-component `Lstat`, regular-file, same-file, permission, and exclusive-create checks. |
| Browser DNS rebinding or cross-site mutation | Exact Host/Origin, loopback remote address, random session, one-use bootstrap, CSRF, SameSite cookie, no CORS. |
| UI fabricates 500 reviews | No bulk endpoint; one case-bound accept/reject per mutation and explicit reviewer UUID. |
| Edit keeps stale approval | Content hash changes and effective decision/reviewer/timestamp are cleared. |
| Crash loses or duplicates a decision | Immutable synced event is authority; expected sequence rejects retry; checkpoint is derived. |
| Ledger tamper or fork | Filename raw hash, previous hash, exact sequence, semantic replay, and freeze-manifest final-hash binding. |
| Holdout is rerun after failure | Consumed marker is written before external content publication and is never rolled back. |
| Passing evidence promotes a reader | No runtime, database, flag, migration, or production API dependency exists. |

## Known limitations

- Deterministic templates prioritize reproducibility and coverage over natural
  language diversity. The human review task owns edits and rejection.
- Rejecting too many candidates in one quota cell can make exact freeze
  impossible. v1 fails closed; it has no automatic supplement generator.
- File permissions and tool state cannot stop the local machine owner from
  bypassing the command and copying Holdout bytes.
- A crash after the consumed marker is durable but before a complete formal
  observation intentionally burns the Holdout and requires a new corpus
  version plus new review/freeze evidence.

## Change history

- 2026-07-29: initial deterministic generator, protected artifacts,
  hash-chained human review, loopback UI, exact freeze, and one-shot Holdout.
