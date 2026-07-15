package jobartifacts

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/files"
)

func TestStoreImageArtifactUsesFileUploadBoundary(t *testing.T) {
	uploader := &fakeUploader{record: files.FileRecord{
		ID:       "file-1",
		MimeType: "image/png",
		ByteSize: 4,
	}}
	service := NewService(uploader)

	artifact, err := service.Store(context.Background(), StoreInput{
		JobID:       "job-1",
		Kind:        KindImage,
		Filename:    "../ignored/generated.png",
		ContentType: "image/png; charset=binary",
		Size:        4,
		Body:        strings.NewReader("data"),
	})

	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if artifact != (Artifact{FileID: "file-1", Purpose: "image", ContentType: "image/png", Size: 4}) {
		t.Fatalf("artifact = %#v", artifact)
	}
	if uploader.input.OriginalFilename != "generated.png" {
		t.Fatalf("filename = %q, want generated.png", uploader.input.OriginalFilename)
	}
	if uploader.input.Purpose != "image" || uploader.input.ClientFileID != "job-1" {
		t.Fatalf("upload input = %#v", uploader.input)
	}
	body, err := io.ReadAll(uploader.input.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "data" {
		t.Fatalf("body = %q, want data", string(body))
	}
}

func TestStoreAudioArtifactGeneratesSafeFilenameAndClientFileID(t *testing.T) {
	uploader := &fakeUploader{record: files.FileRecord{
		ID:       "file-2",
		MimeType: "audio/webm",
		ByteSize: 5,
	}}
	service := NewService(uploader)

	_, err := service.Store(context.Background(), StoreInput{
		JobID:       "job/secret 1",
		Kind:        KindAudio,
		ContentType: "audio/webm",
		Size:        5,
		Body:        strings.NewReader("audio"),
	})

	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if uploader.input.OriginalFilename != "jobsecret1.webm" {
		t.Fatalf("filename = %q, want jobsecret1.webm", uploader.input.OriginalFilename)
	}
	if uploader.input.Purpose != "audio" || uploader.input.ClientFileID != "jobsecret1" {
		t.Fatalf("upload input = %#v", uploader.input)
	}
}

func TestStoreRejectsInvalidArtifactsBeforeUpload(t *testing.T) {
	tests := []struct {
		name  string
		input StoreInput
		want  error
	}{
		{name: "nil body", input: StoreInput{Kind: KindImage, ContentType: "image/png", Size: 1}, want: ErrArtifactBodyRequired},
		{name: "zero size", input: StoreInput{Kind: KindImage, ContentType: "image/png", Size: 0, Body: strings.NewReader("x")}, want: ErrArtifactSizeInvalid},
		{name: "bad kind", input: StoreInput{Kind: Kind("video"), ContentType: "video/mp4", Size: 1, Body: strings.NewReader("x")}, want: ErrArtifactKindInvalid},
		{name: "bad image type", input: StoreInput{Kind: KindImage, ContentType: "audio/webm", Size: 1, Body: strings.NewReader("x")}, want: ErrArtifactTypeInvalid},
		{name: "bad audio type", input: StoreInput{Kind: KindAudio, ContentType: "image/png", Size: 1, Body: strings.NewReader("x")}, want: ErrArtifactTypeInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uploader := &fakeUploader{}
			_, err := NewService(uploader).Store(context.Background(), tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Store() error = %v, want %v", err, tt.want)
			}
			if uploader.called {
				t.Fatal("uploader was called for invalid artifact")
			}
		})
	}
}

func TestStoreRequiresUploader(t *testing.T) {
	_, err := NewService(nil).Store(context.Background(), StoreInput{
		Kind:        KindImage,
		ContentType: "image/png",
		Size:        1,
		Body:        strings.NewReader("x"),
	})
	if !errors.Is(err, ErrUploaderRequired) {
		t.Fatalf("Store() error = %v, want ErrUploaderRequired", err)
	}
}

func TestStoreWrapsUploaderError(t *testing.T) {
	uploader := &fakeUploader{err: errors.New("database down")}
	_, err := NewService(uploader).Store(context.Background(), StoreInput{
		Kind:        KindImage,
		ContentType: "image/png",
		Size:        1,
		Body:        strings.NewReader("x"),
	})
	if err == nil || !strings.Contains(err.Error(), "store job artifact") {
		t.Fatalf("Store() error = %v, want wrapped uploader error", err)
	}
}

type fakeUploader struct {
	called bool
	input  files.UploadInput
	record files.FileRecord
	err    error
}

func (u *fakeUploader) Upload(_ context.Context, input files.UploadInput) (files.FileRecord, error) {
	u.called = true
	u.input = input
	if u.err != nil {
		return files.FileRecord{}, u.err
	}
	record := u.record
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	return record, nil
}
