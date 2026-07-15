package voicejobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
	"neo-chat/mm-chat/backend/internal/providersmoke"
)

func TestLiveOpenAICompatibleVoiceSmoke(t *testing.T) {
	smokeCfg := providersmoke.LoadFromEnv(os.LookupEnv)
	if !smokeCfg.Enabled {
		t.Skip("provider live smoke disabled")
	}
	target, ok := firstLiveVoiceTarget(smokeCfg.Targets)
	if !ok {
		t.Skip("provider live smoke enabled but no voice target configured")
	}
	if err := smokeCfg.Authorize(target); err != nil {
		t.Fatalf("provider live smoke not authorized: %v", err)
	}

	cfg := config.LoadFromEnv(os.LookupEnv)
	executor, err := NewOpenAICompatibleExecutor(OpenAICompatibleExecutorConfig{
		BaseURL: cfg.Provider.BaseURL,
		APIKey:  cfg.Provider.APIKey,
		Timeout: cfg.Provider.Timeout,
	})
	if err != nil {
		t.Fatalf("configure voice executor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), liveVoiceSmokeTimeout(cfg.Provider.Timeout))
	defer cancel()
	switch target.Kind {
	case providersmoke.KindVoiceSynthesize:
		store := &liveVoiceArtifactStore{dir: liveVoiceSmokeOutputDir()}
		service := NewService(
			WithExecutor(executor),
			WithArtifactStore(store),
			WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error { return nil })),
		)
		response, err := service.Synthesize(ctx, SynthesizeRequest{
			Provider: ProviderModel,
			ModelID:  target.ModelID,
			Text:     "Short voice smoke test.",
		})
		if err != nil {
			t.Fatalf("live voice synthesize smoke failed: %v", err)
		}
		if response.FileID == "" || response.Purpose != "audio" || response.Size <= 0 {
			t.Fatalf("audio artifact metadata = %#v", response)
		}
		if len(store.paths) != 1 {
			t.Fatalf("stored paths = %#v, want one output file", store.paths)
		}
		t.Logf("live voice synthesize smoke stored artifact at %s", store.paths[0])
	case providersmoke.KindVoiceTranscribe:
		service := NewService(
			WithExecutor(executor),
			WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error { return nil })),
		)
		response, err := service.Transcribe(ctx, TranscribeRequest{
			Provider:         ProviderModel,
			ModelID:          target.ModelID,
			AudioFilename:    "voice-smoke.wav",
			AudioContentType: "audio/wav",
			AudioSize:        int64(len(silentWAVBytes())),
			Audio:            bytes.NewReader(silentWAVBytes()),
		})
		if err != nil {
			t.Fatalf("live voice transcribe smoke failed: %v", err)
		}
		t.Logf("live voice transcribe smoke returned %d transcript characters", len(response.Text))
	default:
		t.Fatalf("unsupported voice smoke target kind: %s", target.Kind)
	}
}

func firstLiveVoiceTarget(targets []providersmoke.Target) (providersmoke.Target, bool) {
	for _, target := range targets {
		if target.Kind == providersmoke.KindVoiceSynthesize || target.Kind == providersmoke.KindVoiceTranscribe {
			return target, true
		}
	}
	return providersmoke.Target{}, false
}

func liveVoiceSmokeTimeout(providerTimeout time.Duration) time.Duration {
	if providerTimeout > 0 {
		return providerTimeout + 30*time.Second
	}
	return 3 * time.Minute
}

func liveVoiceSmokeOutputDir() string {
	if value := strings.TrimSpace(os.Getenv("MM_CHAT_PROVIDER_LIVE_SMOKE_OUTPUT_DIR")); value != "" {
		return value
	}
	return filepath.Join(os.TempDir(), "mm-chat-provider-smoke")
}

type liveVoiceArtifactStore struct {
	dir   string
	paths []string
}

func (s *liveVoiceArtifactStore) Store(_ context.Context, input jobartifacts.StoreInput) (jobartifacts.Artifact, error) {
	if input.Kind != jobartifacts.KindAudio {
		return jobartifacts.Artifact{}, errors.New("live smoke expected audio artifact")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return jobartifacts.Artifact{}, fmt.Errorf("create live smoke dir: %w", err)
	}
	filename := filepath.Base(input.Filename)
	if filename == "." || filename == string(filepath.Separator) || strings.TrimSpace(filename) == "" {
		filename = "live-voice-smoke.bin"
	}
	path := filepath.Join(s.dir, fmt.Sprintf("%d-%s", len(s.paths)+1, filename))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return jobartifacts.Artifact{}, fmt.Errorf("create live smoke artifact: %w", err)
	}
	size, copyErr := io.Copy(file, input.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return jobartifacts.Artifact{}, fmt.Errorf("write live smoke artifact: %w", copyErr)
	}
	if closeErr != nil {
		return jobartifacts.Artifact{}, fmt.Errorf("close live smoke artifact: %w", closeErr)
	}
	if size <= 0 {
		return jobartifacts.Artifact{}, errors.New("live smoke artifact is empty")
	}
	s.paths = append(s.paths, path)
	return jobartifacts.Artifact{
		FileID:      filepath.Base(path),
		Purpose:     "audio",
		ContentType: input.ContentType,
		Size:        size,
	}, nil
}

func silentWAVBytes() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 0x2c, 0x00, 0x00, 0x00, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 0x10, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
		0x40, 0x1f, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00, 0x02, 0x00, 0x10, 0x00,
		'd', 'a', 't', 'a', 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}
