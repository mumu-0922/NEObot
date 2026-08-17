package httpserver

import (
	"bytes"
	"context"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/files"
)

type chatWorkspaceArtifactPublisher struct {
	service artifactFileService
}

type artifactFileService interface {
	Upload(context.Context, files.UploadInput) (files.FileRecord, error)
	Delete(context.Context, string) error
}

func (publisher chatWorkspaceArtifactPublisher) PublishWorkspaceArtifact(
	ctx context.Context,
	input chat.WorkspaceArtifactPublishInput,
) (chat.WorkspaceArtifact, error) {
	record, err := publisher.service.Upload(ctx, files.UploadInput{
		OriginalFilename: input.FileName,
		MimeType:         input.MimeType,
		Size:             int64(len(input.Body)),
		Purpose:          "export",
		ConversationID:   input.ConversationID,
		Body:             bytes.NewReader(input.Body),
	})
	if err != nil {
		return chat.WorkspaceArtifact{}, err
	}
	return chat.WorkspaceArtifact{
		FileID: record.ID, FileName: record.OriginalFilename,
		MimeType: record.MimeType, Size: record.ByteSize, SHA256: record.SHA256,
	}, nil
}

func (publisher chatWorkspaceArtifactPublisher) DeleteWorkspaceArtifact(
	ctx context.Context,
	fileID string,
) error {
	return publisher.service.Delete(ctx, fileID)
}
