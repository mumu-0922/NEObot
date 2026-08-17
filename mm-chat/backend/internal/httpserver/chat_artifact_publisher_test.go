package httpserver

import (
	"context"
	"io"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/files"
)

func TestChatWorkspaceArtifactPublisherUsesExportFileAuthority(t *testing.T) {
	service := &fakeArtifactFileService{}
	publisher := chatWorkspaceArtifactPublisher{service: service}
	artifact, err := publisher.PublishWorkspaceArtifact(
		context.Background(),
		chat.WorkspaceArtifactPublishInput{
			ConversationID: "11111111-1111-4111-8111-111111111111",
			FileName:       "result.bin", MimeType: "application/octet-stream",
			Body: []byte{0x00, 0xff, 0x01},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if service.upload.Purpose != "export" ||
		service.upload.ConversationID != "11111111-1111-4111-8111-111111111111" ||
		service.upload.OriginalFilename != "result.bin" ||
		service.upload.MimeType != "application/octet-stream" ||
		service.upload.Size != 3 || string(service.body) != string([]byte{0x00, 0xff, 0x01}) {
		t.Fatalf("upload=%#v body=%v", service.upload, service.body)
	}
	if artifact.FileID != "55555555-5555-4555-8555-555555555555" ||
		artifact.FileName != "result.bin" || artifact.Size != 3 {
		t.Fatalf("artifact=%#v", artifact)
	}
	if err := publisher.DeleteWorkspaceArtifact(context.Background(), artifact.FileID); err != nil {
		t.Fatal(err)
	}
	if service.deleted != artifact.FileID {
		t.Fatalf("deleted=%q", service.deleted)
	}
}

type fakeArtifactFileService struct {
	upload  files.UploadInput
	body    []byte
	deleted string
}

func (service *fakeArtifactFileService) Upload(
	_ context.Context,
	input files.UploadInput,
) (files.FileRecord, error) {
	service.upload = input
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return files.FileRecord{}, err
	}
	service.body = body
	return files.FileRecord{
		ID:               "55555555-5555-4555-8555-555555555555",
		OriginalFilename: input.OriginalFilename, MimeType: input.MimeType,
		ByteSize: input.Size, SHA256: "sha256",
	}, nil
}

func (service *fakeArtifactFileService) Delete(_ context.Context, fileID string) error {
	service.deleted = fileID
	return nil
}
