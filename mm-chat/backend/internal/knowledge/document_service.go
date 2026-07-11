package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func (s *Service) CreateDocumentVersion(ctx context.Context, documentID string, input BindDocumentInput) (Document, error) {
	if err := s.requireRepository(); err != nil {
		return Document{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Document{}, err
	}
	documentID, err = normalizeUUID(documentID, "document id")
	if err != nil {
		return Document{}, err
	}
	input.FileID, err = normalizeUUID(input.FileID, "fileId")
	if err != nil {
		return Document{}, err
	}
	input.IdempotencyKey, err = normalizeIdempotencyKey(input.IdempotencyKey)
	if err != nil {
		return Document{}, err
	}
	ids := make([]string, 2)
	for index := range ids {
		ids[index], err = s.newID()
		if err != nil {
			return Document{}, fmt.Errorf("generate document version identity: %w", err)
		}
	}
	sum := sha256.Sum256([]byte(documentID + "\n" + input.FileID))
	return s.repo.CreateDocumentVersion(ctx, CreateDocumentVersionRepositoryInput{
		VersionID: ids[0], JobID: ids[1], DocumentID: documentID,
		ActorUserID: actor.ID, FileID: input.FileID, IdempotencyKey: input.IdempotencyKey,
		RequestHash: hex.EncodeToString(sum[:]), ParseProcessor: "mineru",
	})
}
