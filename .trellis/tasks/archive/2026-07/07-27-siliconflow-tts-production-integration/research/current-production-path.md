# Research: Current Production TTS Path

- Date: 2026-07-27
- Scope: repository runtime, credential, artifact, capability, and frontend
  read-aloud boundaries after the successful SiliconFlow smoke.

## Proven Provider Tuple

- Base URL: `https://api.siliconflow.cn/v1`
- Model: `FunAudioLLM/CosyVoice2-0.5B`
- Voice: `FunAudioLLM/CosyVoice2-0.5B:claire`
- Endpoint: `POST /audio/speech`
- The authorized Go smoke produced a valid MP3 and the owner accepted Chinese
  playback quality.

## Existing Backend Seams

- `voicejobs.OpenAICompatibleExecutor` already emits the correct Bearer-auth
  `model/input/voice` request and returns bounded audio.
- `voicejobs.Service` already requires admitted audit plus artifact storage
  before synthesis and returns only `fileId`, purpose, content type, and size.
- `cmd/api` constructs the Voice service with audit and artifact storage but
  deliberately installs no executor, so production returns
  `VOICE_JOBS_UNAVAILABLE`.
- `httpserver` already registers authenticated `POST /v1/voice/synthesize`.
- Reserved Voice authority supports only `VOICE:ELEVENLABS` and `VOICE:MIMO`.
  Production SiliconFlow requires a new exact `VOICE:SILICONFLOW` identity,
  `config.kind="voice"`, provider match validation, and vault context
  `provider:voice:<userId>:VOICE:SILICONFLOW`.
- Model, Search, and RAG provider credentials are separate authorities and
  must not be reused.

## Existing Frontend Seams

- `MessageItem` already has an explicit read-aloud button and uses
  `synthesizeSpeech`; there is no automatic generation on message arrival.
- In server mode, `capabilities.voice=false` currently blocks hosted TTS, while
  browser `speechSynthesis` remains usable.
- The current single `voice` capability covers both synthesis and
  transcription. Setting it true for a TTS-only rollout would falsely admit
  STT, so the production contract needs separate synthesis/transcription
  capability truth or an equivalent fail-closed distinction.
- The transitional frontend service still calls Next `/api/voice/synthesize`
  and expects a Blob. The Go route returns stored artifact metadata, so server
  mode must call Go, then fetch the actor-authorized file by `fileId` and wrap
  it in the existing disposable audio element.

## Production Risks

- Repeated clicks can create repeated paid calls and orphan stored audio unless
  the production flow defines reuse/idempotency or retention cleanup.
- A provider row must not become active after a syntax-only save. It needs a
  bounded real connection test for the exact model/voice plus a current
  attestation tied to the credential fingerprint and configuration.
- The credential pasted during smoke is considered exposed and must never be
  promoted. Production setup requires a fresh key entered through encrypted
  administrator ingress.
- Runtime must fail closed on missing/disabled/stale provider configuration,
  vault failure, attestation mismatch, or storage/audit failure. Browser
  fallback must remain an explicit product behavior, not a silent server-side
  credential bypass.

## Recommended Architecture

Use the established administrator Provider Settings and AES-GCM vault chain:

```text
fresh admin BYOK ingress
  -> VOICE:SILICONFLOW Postgres row + dedicated Voice vault context
  -> exact bounded CosyVoice2/claire connection test + attestation
  -> single active Voice resolver
  -> OpenAICompatibleExecutor in cmd/api
  -> admitted audit + artifact store
  -> Go artifact metadata
  -> authenticated frontend file fetch + disposable audio playback
```

Keep ASR, custom voice upload/cloning, and reuse of RAG/model credentials out of
scope. Preserve browser TTS as a separate device-local choice.
