package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestSiliconFlowTTSCacheMigrationContract(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("051_siliconflow_tts_cache.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("051_siliconflow_tts_cache.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)
	for _, required := range []string{
		"provider_configs_voice_identity_check",
		"VOICE:SILICONFLOW",
		"FunAudioLLM/CosyVoice2-0.5B:claire",
		"CREATE TABLE tts_audio_cache",
		"UNIQUE (user_id, message_id)",
		"REFERENCES messages(id) ON DELETE RESTRICT",
		"CREATE TABLE tts_audio_cleanup_queue",
		"idx_tts_audio_cache_user_lru",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("051 up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"DROP TABLE IF EXISTS tts_audio_cleanup_queue",
		"DROP TABLE IF EXISTS tts_audio_cache",
		"DROP CONSTRAINT IF EXISTS provider_configs_voice_identity_check",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("051 down migration missing %q", required)
		}
	}
}
