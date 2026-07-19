# Future Voice Provider Reservation Contract

## 1. Scope / Trigger

G11.9F.5 reserves the Postgres/vault identity for a future free or low-cost
hosted Voice integration without enabling one now. The owner explicitly does
not want a VPS-local TTS engine and has not selected a hosted API. Therefore
this slice adds no Voice administrator page, provider request adapter,
connection test, runtime resolver, environment fallback, or quota-consuming
smoke.

This contract becomes the mandatory starting point when ElevenLabs, Mimo, or a
replacement hosted STT/TTS provider is implemented later.

## 2. Signatures

Reserved `provider_configs` identities:

```text
provider_id=VOICE:ELEVENLABS  config.kind=voice  config.voiceProvider=elevenlabs
provider_id=VOICE:MIMO        config.kind=voice  config.voiceProvider=mimo
```

Reserved encryption contexts:

```text
future browser ingress: provider:voice:<elevenlabs|mimo>
vault at rest:          provider:voice:<userId>:<VOICE:PROVIDER>
```

F5 registers **no** `/v1/admin/voice/providers*` route. Existing
`POST /v1/voice/transcribe` and `POST /v1/voice/synthesize` signatures remain
unchanged and fail closed because `cmd/api` still installs no Voice executor.

## 3. Contracts

- `kind="voice"`, `voiceProvider`, and the `VOICE:*` record ID must agree
  exactly; the only reserved providers are `elevenlabs` and `mimo`.
- Model, Search, and RAG readers must reject Voice rows. An empty-kind legacy
  row cannot claim a `VOICE:*` ID or model vault context.
- Current/retained-key `A256GCM` Voice envelopes participate in the common
  exact-plan vault rewrite under the Voice context.
- Legacy browser BYOK Voice rows are blocked rather than guessed or migrated;
  a future administrator flow must accept a fresh ingress envelope.
- Rotation may not create or preserve a Voice connection attestation in F5.
  No provider-specific real-test contract exists yet, so Voice remains
  unavailable regardless of a syntactically valid reserved row.
- No Voice provider Key, model, voice ID, base URL, or reusable credential is
  added to Go/Python/Compose/operator environment authority.
- Browser `speechSynthesis` remains an allowed device-local fallback. It does
  not call `/v1/voice/*`, create a provider row, or close stored-audio parity.

## 4. Validation and Error Matrix

| Condition | Result |
| --- | --- |
| Unknown `voiceProvider` | reserved context rejected |
| `VOICE:ELEVENLABS` with `voiceProvider=mimo` | reserved context rejected |
| `VOICE:*` with empty/model kind | model reader and rewrite reject it |
| Legacy BYOK Voice envelope | `blocked_rows` increments; execute is blocked |
| Retained-key Voice vault envelope | rotates under the same Voice context |
| Voice envelope copied to model/Search/RAG/other Voice record | AES-GCM context mismatch |
| Reserved Voice row during rotation | no attestation is invented |
| Voice API call in the current runtime | fail-closed `VOICE_JOBS_UNAVAILABLE` |

## 5. Good / Base / Bad Cases

- Good: a future retained-key Voice envelope rotates and decrypts only under
  `provider:voice:<userId>:<matching-record>`, while Voice remains unavailable.
- Base: no Voice rows exist; public config reports Voice unavailable and the
  existing executor seam consumes no quota.
- Bad: reuse `SERVER_DEFAULT`, a model Key, or a `provider:model:*` context to
  enable Voice.
- Bad: set `enabled=true` or copy an old fingerprint and treat that as a real
  Voice connection test.

## 6. Tests Required

- exact provider/kind/record matching and unknown/mismatch rejection;
- Model/Search/RAG exclusion for `VOICE:*` rows;
- retained-key Voice rotation plus fresh active-key decryption;
- cross-context and cross-Voice-record copy rejection;
- legacy Voice BYOK blocked with zero changed rows;
- invalid/unimplemented Voice attestation non-promotion;
- default `/v1/voice/*` fail-closed behavior and zero network calls;
- clean-copy scans proving no Voice reusable Key environment authority.

## 7. Wrong vs Correct

Wrong:

```text
DEFAULT_ELEVENLABS_API_KEY -> model provider resolver -> Voice executor
```

Correct future chain:

```text
administrator BYOK -> VOICE:* Postgres/vault row
  -> provider-specific bounded real test
  -> exact attestation
  -> dedicated Voice resolver/executor
  -> stored audio artifact metadata
```

Until that complete chain exists, keep the current server Voice runtime
unavailable rather than adding an environment shortcut.
