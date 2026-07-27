REVOKE SELECT, INSERT, UPDATE, DELETE
  ON TABLE tts_audio_cache, tts_audio_cleanup_queue
  FROM go_api_runtime;
