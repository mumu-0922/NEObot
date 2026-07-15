package imagejobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

func TestServiceAuditsUnavailableImageGenerationWithoutPrompt(t *testing.T) {
	var got jobaudit.Event
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
		got = event
		return nil
	})))
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	_, err := service.Generate(ctx, GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "paint a private scene",
		Count:    1,
	})

	if !errors.Is(err, ErrImageJobsUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrImageJobsUnavailable", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindImageGenerate,
		Status:     jobaudit.StatusUnavailable,
		UserID:     "user-1",
		ProviderID: "openai",
		ModelID:    "gpt-image-1",
		Reason:     "IMAGE_JOBS_UNAVAILABLE",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
}

func TestServiceFailsClosedWhenImageAuditUnavailable(t *testing.T) {
	service := NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return errors.New("audit sink down")
	})))

	_, err := service.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "paint",
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrAuditUnavailable", err)
	}
}

func TestServiceGenerateRequiresArtifactStoreBeforeExecutor(t *testing.T) {
	var got jobaudit.Event
	executor := &fakeImageExecutor{}
	service := NewService(
		WithExecutor(executor),
		WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
			got = event
			return nil
		})),
	)

	_, err := service.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "private prompt",
	})

	if !errors.Is(err, ErrImageArtifactStoreUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrImageArtifactStoreUnavailable", err)
	}
	if executor.called {
		t.Fatal("executor was called before artifact store was configured")
	}
	if got.Reason != "IMAGE_ARTIFACT_STORE_UNAVAILABLE" || got.ModelID != "gpt-image-1" {
		t.Fatalf("audit event = %#v", got)
	}
}

func TestServiceDoesNotCallExecutorWithoutAdmissionAuditRecorder(t *testing.T) {
	executor := &fakeImageExecutor{}
	service := NewService(WithExecutor(executor), WithArtifactStore(&fakeArtifactStore{}))

	_, err := service.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "private prompt",
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrAuditUnavailable", err)
	}
	if executor.called {
		t.Fatal("executor was called without an admission audit recorder")
	}
}

func TestServiceDoesNotCallExecutorWhenAdmissionAuditUnavailable(t *testing.T) {
	executor := &fakeImageExecutor{}
	service := NewService(
		WithExecutor(executor),
		WithArtifactStore(&fakeArtifactStore{}),
		WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
			return errors.New("audit sink down")
		})),
	)

	_, err := service.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "private prompt",
	})

	if !errors.Is(err, jobaudit.ErrAuditUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrAuditUnavailable", err)
	}
	if executor.called {
		t.Fatal("executor was called after admission audit failed")
	}
}

func TestServiceAuditsAdmittedImageGenerationWithoutPrompt(t *testing.T) {
	var got jobaudit.Event
	executor := &fakeImageExecutor{result: GenerateResult{Images: []GeneratedImageResult{
		{
			JobID:       "job-1",
			Filename:    "image.png",
			ContentType: "image/png",
			Size:        5,
			Body:        strings.NewReader("image"),
		},
	}}}
	store := &fakeArtifactStore{artifacts: []jobartifacts.Artifact{{
		FileID:      "file-1",
		Purpose:     "image",
		ContentType: "image/png",
		Size:        5,
	}}}
	service := NewService(
		WithExecutor(executor),
		WithArtifactStore(store),
		WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
			got = event
			return nil
		})),
	)
	ctx := auth.WithUser(context.Background(), auth.User{ID: "user-1"})

	_, err := service.Generate(ctx, GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "private prompt",
		Count:    1,
	})

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	want := jobaudit.Event{
		Kind:       jobaudit.KindImageGenerate,
		Status:     jobaudit.StatusAdmitted,
		UserID:     "user-1",
		ProviderID: "openai",
		ModelID:    "gpt-image-1",
	}
	if got != want {
		t.Fatalf("audit event = %#v, want %#v", got, want)
	}
}

func TestServiceGenerateStoresExecutorImageArtifacts(t *testing.T) {
	executor := &fakeImageExecutor{result: GenerateResult{
		Images: []GeneratedImageResult{
			{
				JobID:       "job-1",
				Filename:    "first.png",
				ContentType: "image/png",
				Size:        5,
				Body:        strings.NewReader("first"),
			},
			{
				JobID:       "job-2",
				Filename:    "second.webp",
				ContentType: "image/webp",
				Size:        6,
				Body:        strings.NewReader("second"),
			},
		},
		Message: "stored",
	}}
	store := &fakeArtifactStore{artifacts: []jobartifacts.Artifact{
		{FileID: "file-1", Purpose: "image", ContentType: "image/png", Size: 5},
		{FileID: "file-2", Purpose: "image", ContentType: "image/webp", Size: 6},
	}}
	service := NewService(
		WithExecutor(executor),
		WithArtifactStore(store),
		WithAuditRecorder(noopAuditRecorder()),
	)

	response, err := service.Generate(context.Background(), GenerateRequest{
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-image-1"},
		Prompt:   "private prompt",
		Count:    2,
	})

	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !executor.called {
		t.Fatal("executor was not called")
	}
	if executor.request.Prompt != "private prompt" || executor.request.Count != 2 {
		t.Fatalf("executor request = %#v", executor.request)
	}
	wantImages := []GeneratedImage{
		{FileID: "file-1", Purpose: "image", ContentType: "image/png", Size: 5},
		{FileID: "file-2", Purpose: "image", ContentType: "image/webp", Size: 6},
	}
	if response.Message != "stored" || len(response.Images) != len(wantImages) {
		t.Fatalf("response = %#v", response)
	}
	for index, want := range wantImages {
		if response.Images[index] != want {
			t.Fatalf("image[%d] = %#v, want %#v", index, response.Images[index], want)
		}
	}
	if len(store.inputs) != 2 {
		t.Fatalf("stored artifacts = %d, want 2", len(store.inputs))
	}
	if store.inputs[0].Kind != jobartifacts.KindImage ||
		store.inputs[0].JobID != "job-1" ||
		store.inputs[0].Filename != "first.png" ||
		store.inputs[0].ContentType != "image/png" ||
		store.inputs[0].Size != 5 {
		t.Fatalf("first artifact input = %#v", store.inputs[0])
	}
	if store.inputs[1].Kind != jobartifacts.KindImage ||
		store.inputs[1].JobID != "job-2" ||
		store.inputs[1].Filename != "second.webp" ||
		store.inputs[1].ContentType != "image/webp" ||
		store.inputs[1].Size != 6 {
		t.Fatalf("second artifact input = %#v", store.inputs[1])
	}
	if body := readAllString(t, store.inputs[0].Body); body != "first" {
		t.Fatalf("first body = %q, want first", body)
	}
	if body := readAllString(t, store.inputs[1].Body); body != "second" {
		t.Fatalf("second body = %q, want second", body)
	}
}

type fakeImageExecutor struct {
	called  bool
	request GenerateRequest
	result  GenerateResult
	err     error
}

func (e *fakeImageExecutor) Generate(_ context.Context, request GenerateRequest) (GenerateResult, error) {
	e.called = true
	e.request = request
	if e.err != nil {
		return GenerateResult{}, e.err
	}
	return e.result, nil
}

type fakeArtifactStore struct {
	inputs    []jobartifacts.StoreInput
	artifacts []jobartifacts.Artifact
	err       error
}

func (s *fakeArtifactStore) Store(_ context.Context, input jobartifacts.StoreInput) (jobartifacts.Artifact, error) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return jobartifacts.Artifact{}, s.err
	}
	if len(s.artifacts) < len(s.inputs) {
		return jobartifacts.Artifact{}, errors.New("missing artifact")
	}
	return s.artifacts[len(s.inputs)-1], nil
}

func noopAuditRecorder() jobaudit.Recorder {
	return jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return nil
	})
}

func readAllString(t *testing.T, reader io.Reader) string {
	t.Helper()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}
