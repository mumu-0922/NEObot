package imagejobs

import (
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

const liveImageSmokeOutputDirEnv = "MM_CHAT_PROVIDER_LIVE_SMOKE_OUTPUT_DIR"

func TestLiveOpenAICompatibleImageGenerationSmoke(t *testing.T) {
	smokeCfg := providersmoke.LoadFromEnv(os.LookupEnv)
	if !smokeCfg.Enabled {
		t.Skip("provider live smoke disabled")
	}
	target, ok := firstLiveImageTarget(smokeCfg.Targets)
	if !ok {
		t.Fatal("provider live smoke enabled but no image.generate target configured")
	}
	if err := smokeCfg.Authorize(target); err != nil {
		t.Fatalf("provider live smoke not authorized: %v", err)
	}

	cfg := config.LoadFromEnv(os.LookupEnv)
	executor, err := NewOpenAICompatibleExecutor(OpenAICompatibleExecutorConfig{
		BaseURL: strings.TrimSpace(os.Getenv("MM_CHAT_PROVIDER_LIVE_SMOKE_BASE_URL")),
		APIKey:  strings.TrimSpace(os.Getenv("MM_CHAT_PROVIDER_LIVE_SMOKE_API_KEY")),
		Timeout: cfg.Provider.Timeout,
	})
	if err != nil {
		t.Fatalf("configure image executor: %v", err)
	}

	store := &liveArtifactStore{dir: liveSmokeOutputDir()}
	service := NewService(
		WithExecutor(executor),
		WithArtifactStore(store),
		WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error { return nil })),
	)

	ctx, cancel := context.WithTimeout(context.Background(), liveSmokeTimeout(cfg.Provider.Timeout))
	defer cancel()
	response, err := service.Generate(ctx, GenerateRequest{
		ModelRef: ModelRef{ProviderID: target.ProviderID, ModelID: target.ModelID},
		Prompt:   "Generate a simple 64x64 abstract smoke-test icon with no text.",
		Size:     strings.TrimSpace(os.Getenv("MM_CHAT_PROVIDER_LIVE_SMOKE_IMAGE_SIZE")),
		Count:    1,
	})
	if err != nil {
		t.Fatalf("live image smoke failed: %v", err)
	}
	if len(response.Images) != 1 {
		t.Fatalf("images = %d, want 1", len(response.Images))
	}
	if response.Images[0].FileID == "" || response.Images[0].Purpose != "image" || response.Images[0].Size <= 0 {
		t.Fatalf("image artifact metadata = %#v", response.Images[0])
	}
	if len(store.paths) != 1 {
		t.Fatalf("stored paths = %#v, want one output file", store.paths)
	}
	t.Logf("live image smoke stored artifact at %s", store.paths[0])
}

func firstLiveImageTarget(targets []providersmoke.Target) (providersmoke.Target, bool) {
	for _, target := range targets {
		if target.Kind == providersmoke.KindImageGenerate {
			return target, true
		}
	}
	return providersmoke.Target{}, false
}

func liveSmokeTimeout(providerTimeout time.Duration) time.Duration {
	if providerTimeout > 0 {
		return providerTimeout + 30*time.Second
	}
	return 3 * time.Minute
}

func liveSmokeOutputDir() string {
	if value := strings.TrimSpace(os.Getenv(liveImageSmokeOutputDirEnv)); value != "" {
		return value
	}
	return filepath.Join(os.TempDir(), "mm-chat-provider-smoke")
}

type liveArtifactStore struct {
	dir   string
	paths []string
}

func (s *liveArtifactStore) Store(_ context.Context, input jobartifacts.StoreInput) (jobartifacts.Artifact, error) {
	if input.Kind != jobartifacts.KindImage {
		return jobartifacts.Artifact{}, errors.New("live smoke expected image artifact")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return jobartifacts.Artifact{}, fmt.Errorf("create live smoke dir: %w", err)
	}
	filename := filepath.Base(input.Filename)
	if filename == "." || filename == string(filepath.Separator) || strings.TrimSpace(filename) == "" {
		filename = "live-image-smoke.bin"
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
		Purpose:     "image",
		ContentType: input.ContentType,
		Size:        size,
	}, nil
}
