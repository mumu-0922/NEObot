# Chat Source Fusion Contract

## 1. Scope / Trigger

G11.9G closes the user-enabled Auto flow across selected private Knowledge,
optional public Web Search, and ordinary model reasoning. This contract applies
when Go prepares one chat generation after conversation Knowledge binding and
the `useSearch` toggle have been resolved.

## 2. Signatures

The internal Router consumes only bounded runtime facts:

```text
question + searchEnabled + Knowledge outcome/evidence presence
  -> questionClass + authority + searchRequested + reason
```

Stable diagnostic values:

```text
questionClass = current_public | knowledge | general
authority     = mixed | knowledge | web | model
searchReason  = disabled | current_public | knowledge_sufficient |
                knowledge_unavailable
```

No new public route, request field, provider selection, or database schema is
introduced. The existing conversation Knowledge binding and `config.useSearch`
remain the only user authorities.

## 3. Contracts

- `useSearch=false` always prevents Search resolution and provider I/O.
- With admitted Knowledge, current/public questions may use both Knowledge and
  Web; non-current questions use Knowledge without unnecessary Web I/O.
- Without admitted Knowledge, enabled Search may supply public context; a
  normal Knowledge miss is not displayed as an error.
- Private/internal claims prefer `[K]`; current public claims prefer `[W]`;
  model synthesis receives no fabricated source identity.
- If `[K]` and `[W]` conflict, the answer must state the different scopes or
  timestamps and cite both instead of silently choosing one.
- Exactly one already-active Search provider is resolved. There is no provider
  fan-out or fallback.
- Diagnostics contain enums, IDs, counts, scores, stages, timings, provider,
  and degradation reason only. They never contain query text, full source text,
  credentials, ciphertext, or signed capabilities.
- When mixed routing selects external Web, the outbound query may append at
  most two already-admitted Knowledge snippets with a combined 512-byte cap.
  This happens only under the user's enabled Search toggle; hashes, locators,
  IDs, and the derived query are never persisted to diagnostics.

## 4. Validation and Error Matrix

| Condition | Result |
| --- | --- |
| Search disabled | no Search resolver/provider call |
| Knowledge ready, no current/public intent | Knowledge + model; Web skipped |
| Current/public intent with Knowledge ready | mixed Knowledge/Web plan |
| Search enabled and Knowledge miss/unbound | Web + model plan |
| Knowledge dependency failure | model continues; optional Web may run |
| External Web failure | model continues with available Knowledge; degraded metadata |
| Built-in capability mismatch | degraded metadata; no cross-provider fallback |
| Built-in startup fails before output | retry same model once without built-in Web |
| Neither evidence source available | ordinary model answer |
| `[K]`/`[W]` conflict | present both scopes with both markers |

## 5. Good / Base / Bad Cases

- Good: a bound internal document answers a stable internal question with
  `[K1]`; enabled Search is not charged unnecessarily.
- Good: a current public question combines internal context `[K1]` with a live
  public source `[W1]` and explains any conflict.
- Base: no Knowledge and Search disabled; generation is an ordinary model
  answer with no empty source card.
- Bad: Search runs when the toggle is off, a provider error aborts an otherwise
  useful Auto answer, or a raw private query/source is written to diagnostics.

## 6. Tests Required

- pure Router matrix for disabled, Knowledge-only, Web-only, mixed, and neither;
- handler assertions proving skipped Search produces zero resolver/provider I/O;
- external and built-in Search failure degradation without provider fallback;
- prompt assertions for source authority and conflict disclosure;
- successful mixed model-built-in Search plus same-model startup degradation;
- completed/cancelled/failed message persistence with bounded `[K]`/`[W]`
  artifacts and redacted diagnostics;
- reload and frontend citation-card interaction;
- live Knowledge-only, Web-only, both, neither, failure, restart, and clean-copy
  smoke with temporary artifacts removed.

## 7. Wrong vs Correct

Wrong:

```text
useSearch=true -> always search first -> abort chat on Web failure
```

Correct:

```text
Knowledge decision -> deterministic source Router -> optional one-provider Web
  -> source-aware prompt -> model -> persisted [K]/[W] + redacted diagnostics
```

The frontend renders Knowledge and Web citation cards independently. It adds
one compact localized notice only for an allowlisted Web degradation reason;
disabled, skipped, and no-result lanes render no empty status card.
