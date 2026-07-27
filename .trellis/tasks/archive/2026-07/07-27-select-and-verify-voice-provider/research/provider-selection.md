# Research: Hosted Voice Provider Selection

- Query: Which hosted provider best closes the remaining G6 Voice/TTS smoke
  gate with the least protocol and security drift?
- Scope: mixed repository and official-provider documentation
- Date: 2026-07-27

## Repository Constraints

- Go already implements Bearer-authenticated `POST /audio/speech` with JSON
  `model`, `input`, and `voice`, plus multipart
  `POST /audio/transcriptions` with `file` and `model`.
- Provider calls are allowed only behind exact live-smoke authorization,
  admitted audit, and synthesis artifact storage gates.
- Production runtime has no Voice resolver by design. Existing reserved vault
  identities are `VOICE:ELEVENLABS` and `VOICE:MIMO`; provider identity and
  credential context must never be borrowed from model/Search/RAG.
- Browser-local ElevenLabs/Mimo routes are transitional local-mode behavior,
  not authorization to enable the Go server path.

## Candidate A: SiliconFlow (Recommended for Smoke-First MVP)

Official evidence:

- `https://docs.siliconflow.cn/cn/api-reference/audio/create-speech`
- `https://docs.siliconflow.cn/cn/api-reference/audio/create-audio-transcriptions`
- `https://docs.siliconflow.cn/cn/userguide/capabilities/text-to-speech`
- The official sitemap reports the audio API pages updated on 2026-01-28.

Fit:

- Exact endpoint shapes `/audio/speech` and `/audio/transcriptions` match the
  existing Go executor.
- TTS supports binary `mp3`, `opus`, `wav`, and `pcm`; system voices and custom
  voices are available.
- `FunAudioLLM/CosyVoice2-0.5B` covers Chinese, English, Japanese, Korean, and
  multiple Chinese dialects; `fnlp/MOSS-TTSD-v0.5` covers expressive Chinese
  and English dialogue.
- STT exposes `FunAudioLLM/SenseVoiceSmall` and `TeleAI/TeleSpeechASR` through
  the compatible transcription endpoint.
- The project already operates SiliconFlow for RAG, which reduces vendor and
  network novelty, but the Voice key/context must remain dedicated.

Authenticated catalog evidence supplied by the owner on 2026-07-27:

- `FunAudioLLM/CosyVoice2-0.5B`: CNY 0.05 per 1,000 UTF-8 input bytes;
  model-specific rate limit currently shown as unrestricted.
- `fnlp/MOSS-TTSD-v0.5`: the same CNY 0.05 per 1,000 UTF-8 input bytes and
  currently unrestricted model-specific rate limit.
- `TeleAI/TeleSpeechASR`: CNY 0.00 under the account's current online-inference
  price, with no model-specific rate limit shown.
- `FunAudioLLM/SenseVoiceSmall`: CNY 0.00 under the same current free-ASR
  billing item, with no model-specific rate limit shown.
- `IndexTeam/IndexTTS2` is marked deprecated and is excluded.

The catalog labels the billable TTS metric as UTF-8 input bytes. Therefore
1,000 ASCII characters cost about CNY 0.05, while 1,000 ordinary Chinese Han
characters are roughly 3,000 UTF-8 bytes and therefore about CNY 0.15 before
punctuation or mixed-language variation. Final billing remains the provider's
metered account record.

Zero-priced ASR means provider inference is currently billed at zero for this
account/model. It does not mean anonymous or permanently unlimited service:
requests still require a Key, obey API upload/time bounds and platform-wide
controls, and may be repriced or withdrawn. The free ASR models convert audio
to text; they cannot replace paid TTS audio generation.

Costs/risks:

- Public API docs retrieved here do not expose a stable price table; current
  account pricing and free-credit availability must be confirmed in the
  provider console before the live attempt.
- The simplified `create-speech` endpoint schema retrieved on 2026-07-27 lists
  only `fnlp/MOSS-TTSD-v0.5` in its model enum, while the capability guide
  describes `FunAudioLLM/CosyVoice2-0.5B`. The owner's authenticated model
  catalog screenshot from the same date resolves this conflict: it visibly
  lists both TTS models, alongside `TeleAI/TeleSpeechASR` and
  `FunAudioLLM/SenseVoiceSmall`; `IndexTeam/IndexTTS2` is marked deprecated.
  The screenshot itself is not copied into Git because it contains personal
  account chrome; only this redacted factual observation is retained.
- Production enablement requires adding `VOICE:SILICONFLOW` reservation,
  administrator ingress/test, vault context, attestation, resolver, and runtime
  wiring. The direct harness smoke does not authorize skipping that chain.
- The TTS API documents `stream=true` as a default. The first smoke must verify
  that the existing executor's bounded body reader receives a valid audio file
  for the chosen request shape.
- The API requires a provider-qualified voice such as
  `FunAudioLLM/CosyVoice2-0.5B:claire`. The current Go live harness leaves the
  executor on its OpenAI default `alloy`, so it needs an explicit smoke-only
  voice input before the call can succeed.

## Candidate B: ElevenLabs

Official evidence:

- `https://elevenlabs.io/docs/api-reference/text-to-speech/convert`
- `https://elevenlabs.io/docs/api-reference/speech-to-text/convert`
- `https://elevenlabs.io/pricing/api`

Fit:

- Mature TTS and STT, multilingual models, explicit audio formats, and the
  existing `VOICE:ELEVENLABS` reservation plus transitional frontend adapter.
- The dated pricing page retrieved on 2026-07-27 lists included usage and
  pay-as-you-go rates, including roughly USD 0.05/1K characters for Flash/Turbo,
  USD 0.10/1K for Multilingual, and USD 0.22/hour for Scribe v2.

Costs/risks:

- ElevenLabs uses provider-specific `/v1/text-to-speech/{voiceId}` and
  `/v1/speech-to-text` paths with `xi-api-key`, so the existing Go
  OpenAI-compatible executor cannot be used unchanged.
- Highest confidence/quality option, but more backend adapter work and less
  aligned with the owner's free/simple-first preference.

## Candidate C: Xiaomi MiMo

Repository evidence:

- Transitional frontend routes call
  `https://api.xiaomimimo.com/v1/chat/completions` with `api-key` and models
  `mimo-v2.5-tts` / `mimo-v2.5-asr`.
- `VOICE:MIMO` is already reserved, and the frontend includes Chinese and
  English voice IDs.

Costs/risks:

- The official documentation site is a JavaScript application and could not be
  converted into stable API/pricing evidence by the available documentation
  fetcher on 2026-07-27.
- Its chat-completions audio envelope is not compatible with the existing Go
  `/audio/*` executor, so it requires a dedicated adapter.
- Defer until official wire, quota, retention, and pricing documentation can be
  captured reproducibly.

## Feasible Approaches

### A. Smoke-first SiliconFlow (Recommended)

- Freeze the account-catalog-backed TTS target
  `voice.synthesize:siliconflow:FunAudioLLM/CosyVoice2-0.5B` plus one explicit
  system voice, initially `FunAudioLLM/CosyVoice2-0.5B:claire`.
- Run the existing direct live harness with a dedicated one-off key and exact
  authorization values.
- Record audio metadata/path only; keep normal Voice runtime unavailable.
- Lowest implementation risk and separates protocol proof from production
  credential architecture.

### B. Full SiliconFlow production wiring

- Add the Voice reservation/vault/admin/test/attestation/resolver chain, wire
  `cmd/api`, reopen frontend server capability, then smoke the public route.
- Closes stored server Voice parity in one Task but crosses database, backend,
  frontend, security, deployment, and live-provider boundaries.

### C. ElevenLabs provider-specific production path

- Preserve the existing reservation and build a dedicated Go adapter plus
  administrator/vault/runtime wiring.
- Best mature-provider evidence, but greater code and ongoing cost than the
  smoke-first goal.

## Recommendation

Choose Approach A now. It can prove endpoint compatibility, Chinese audio
quality, artifact handling, and authorization safety before committing to a
new production Voice identity. If the audio/cost check passes, create a
separate production-wiring Task with `VOICE:SILICONFLOW`; if it fails, retain
the current fail-closed runtime and reconsider ElevenLabs.

For bidirectional Voice coverage, use the lowest-cost pair rather than forcing
one model to do both jobs:

- TTS: `FunAudioLLM/CosyVoice2-0.5B` with a provider-qualified preset voice;
- STT: `FunAudioLLM/SenseVoiceSmall` for low-latency general Chinese/English
  transcription; prefer `TeleAI/TeleSpeechASR` only when broad Chinese-dialect
  recognition is the dominant requirement.

Authorize and record the two live targets independently so a free STT success
cannot conceal a paid TTS failure or vice versa.

## Caveats / Not Found

- No provider key was read or discovered; live execution remains blocked until
  the owner supplies a one-off credential through the established smoke env.
- Stable public SiliconFlow pricing and reproducible MiMo official API docs
  were not found in the fetched official pages and must not be guessed.
