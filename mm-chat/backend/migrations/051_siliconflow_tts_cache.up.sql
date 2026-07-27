ALTER TABLE provider_configs
  ADD CONSTRAINT provider_configs_voice_identity_check CHECK (
    (
      provider_id NOT LIKE 'VOICE:%'
      AND COALESCE(config->>'kind', '') <> 'voice'
    )
    OR (
      provider_id = 'VOICE:ELEVENLABS'
      AND config->>'kind' = 'voice'
      AND config->>'voiceProvider' = 'elevenlabs'
    )
    OR (
      provider_id = 'VOICE:MIMO'
      AND config->>'kind' = 'voice'
      AND config->>'voiceProvider' = 'mimo'
    )
    OR (
      provider_id = 'VOICE:SILICONFLOW'
      AND config->>'kind' = 'voice'
      AND config->>'voiceProvider' = 'siliconflow'
      AND config->>'baseUrl' = 'https://api.siliconflow.cn/v1'
      AND config->>'voiceModel' = 'FunAudioLLM/CosyVoice2-0.5B'
      AND config->>'voiceId' = 'FunAudioLLM/CosyVoice2-0.5B:claire'
    )
  );

CREATE TABLE tts_audio_cache (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  message_id UUID NOT NULL REFERENCES messages(id) ON DELETE RESTRICT,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE RESTRICT,
  text_sha256 TEXT NOT NULL,
  source_updated_at TIMESTAMPTZ NOT NULL,
  provider_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  voice_id TEXT NOT NULL,
  content_type TEXT NOT NULL,
  byte_size BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT tts_audio_cache_user_message_unique UNIQUE (user_id, message_id),
  CONSTRAINT tts_audio_cache_file_unique UNIQUE (file_id),
  CONSTRAINT tts_audio_cache_text_sha256_check CHECK (text_sha256 ~ '^[0-9a-f]{64}$'),
  CONSTRAINT tts_audio_cache_provider_not_blank CHECK (length(trim(provider_id)) > 0),
  CONSTRAINT tts_audio_cache_model_not_blank CHECK (length(trim(model_id)) > 0),
  CONSTRAINT tts_audio_cache_voice_not_blank CHECK (length(trim(voice_id)) > 0),
  CONSTRAINT tts_audio_cache_content_type_audio CHECK (content_type LIKE 'audio/%'),
  CONSTRAINT tts_audio_cache_byte_size_positive CHECK (byte_size > 0),
  CONSTRAINT tts_audio_cache_timestamps_order CHECK (
    updated_at >= created_at AND last_accessed_at >= created_at
  )
);

CREATE INDEX idx_tts_audio_cache_user_lru
  ON tts_audio_cache(user_id, last_accessed_at ASC, created_at ASC);
CREATE INDEX idx_tts_audio_cache_message
  ON tts_audio_cache(message_id);

CREATE TABLE tts_audio_cleanup_queue (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  file_id UUID NOT NULL,
  reason TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  claimed_at TIMESTAMPTZ,
  claim_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT tts_audio_cleanup_queue_file_unique UNIQUE (file_id),
  CONSTRAINT tts_audio_cleanup_queue_reason_check CHECK (
    reason IN ('replaced', 'expired', 'lru', 'source_deleted', 'orphaned', 'rollback')
  ),
  CONSTRAINT tts_audio_cleanup_queue_attempts_non_negative CHECK (attempts >= 0),
  CONSTRAINT tts_audio_cleanup_queue_claim_pair CHECK (
    (claimed_at IS NULL AND claim_id IS NULL)
    OR (claimed_at IS NOT NULL AND claim_id IS NOT NULL)
  ),
  CONSTRAINT tts_audio_cleanup_queue_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_tts_audio_cleanup_queue_available
  ON tts_audio_cleanup_queue(created_at ASC)
  WHERE claimed_at IS NULL;
CREATE INDEX idx_tts_audio_cleanup_queue_stale_claim
  ON tts_audio_cleanup_queue(claimed_at ASC)
  WHERE claimed_at IS NOT NULL;
