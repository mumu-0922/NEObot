DROP INDEX IF EXISTS idx_tts_audio_cleanup_queue_stale_claim;
DROP INDEX IF EXISTS idx_tts_audio_cleanup_queue_available;
DROP TABLE IF EXISTS tts_audio_cleanup_queue;

DROP INDEX IF EXISTS idx_tts_audio_cache_message;
DROP INDEX IF EXISTS idx_tts_audio_cache_user_lru;
DROP TABLE IF EXISTS tts_audio_cache;

ALTER TABLE provider_configs
  DROP CONSTRAINT IF EXISTS provider_configs_voice_identity_check;
