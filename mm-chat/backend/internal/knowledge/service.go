package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/storage"
	"neo-chat/mm-chat/backend/internal/teams"
)

const (
	defaultPageLimit                  = 50
	maximumPageLimit                  = 100
	maximumCollectionNameRunes        = 100
	maximumCollectionNameBytes        = 256
	maximumCollectionDescriptionRunes = 2000
	maximumCollectionDescriptionBytes = 8 << 10
	maximumIdempotencyBytes           = 128
	collectionCursorResource          = "knowledge_collections"
	collectionCursorSort              = "created_at:desc,id:desc"
	documentCursorResource            = "knowledge_documents"
	documentCursorSort                = "created_at:desc,id:desc"
)

var validIcons = map[string]struct{}{
	"Folder": {}, "Atom": {}, "BookText": {}, "Microscope": {},
	"Cat": {}, "ChartLine": {}, "ChessKnight": {}, "CodeXml": {},
	"Coffee": {}, "GraduationCap": {}, "MessagesSquare": {}, "Archive": {},
}

var validColors = map[string]struct{}{
	"blue": {}, "purple": {}, "green": {}, "orange": {},
	"red": {}, "pink": {}, "cyan": {}, "gray": {},
}

type Service struct {
	repo        Repository
	cursorCodec *teams.CursorCodec
	newID       func() (string, error)
	objectStore storage.ObjectStore
}

type ServiceOption func(*Service)

func WithCursorCodec(codec *teams.CursorCodec) ServiceOption {
	return func(service *Service) { service.cursorCodec = codec }
}

func WithIDGenerator(generator func() (string, error)) ServiceOption {
	return func(service *Service) {
		if generator != nil {
			service.newID = generator
		}
	}
}

func WithObjectStore(store storage.ObjectStore) ServiceOption {
	return func(service *Service) { service.objectStore = store }
}

func NewService(repo Repository, options ...ServiceOption) *Service {
	service := &Service{repo: repo, newID: newUUID}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) CreateCollection(ctx context.Context, input CreateCollectionInput) (Collection, error) {
	if err := s.requireRepository(); err != nil {
		return Collection{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Collection{}, err
	}
	normalized, err := normalizeCreateInput(input)
	if err != nil {
		return Collection{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Collection{}, fmt.Errorf("generate collection id: %w", err)
	}
	requestHash, err := hashCreateInput(normalized)
	if err != nil {
		return Collection{}, err
	}
	return s.repo.CreateCollection(ctx, CreateCollectionRepositoryInput{
		ID:                id,
		ActorUserID:       actor.ID,
		Name:              normalized.Name,
		Description:       normalized.Description,
		Icon:              normalized.Icon,
		Color:             normalized.Color,
		Scope:             normalized.Scope,
		TeamID:            normalized.TeamID,
		IdempotencyKey:    normalized.IdempotencyKey,
		CreateRequestHash: requestHash,
	})
}

func (s *Service) ListCollections(ctx context.Context, input ListCollectionsInput) (ApiPage[Collection], error) {
	if err := s.requireRepository(); err != nil {
		return ApiPage[Collection]{}, err
	}
	if s == nil || s.cursorCodec == nil {
		return ApiPage[Collection]{}, ErrCursorCodecRequired
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ApiPage[Collection]{}, err
	}
	filter, err := normalizeListInput(input)
	if err != nil {
		return ApiPage[Collection]{}, err
	}
	after, err := s.decodeCursor(filter.Cursor, actor.ID, filter.Scope, filter.TeamID)
	if err != nil {
		return ApiPage[Collection]{}, err
	}
	result, err := s.repo.ListCollections(ctx, ListCollectionsRepositoryInput{
		ActorUserID: actor.ID,
		Scope:       filter.Scope,
		TeamID:      filter.TeamID,
		Limit:       filter.Limit,
		After:       after,
	})
	if err != nil {
		return ApiPage[Collection]{}, err
	}
	page := ApiPage[Collection]{Items: result.Items}
	if result.HasMore && len(result.Items) > 0 {
		last := result.Items[len(result.Items)-1]
		page.NextCursor, err = s.cursorCodec.Encode(teams.Cursor{
			Resource:     collectionCursorResource,
			UserID:       actor.ID,
			FilterDigest: collectionFilterDigest(filter.Scope, filter.TeamID),
			Sort:         collectionCursorSort,
			Values:       []string{last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID},
		})
		if err != nil {
			return ApiPage[Collection]{}, fmt.Errorf("encode collection cursor: %w", err)
		}
	}
	return page, nil
}

func (s *Service) GetCollection(ctx context.Context, collectionID string) (Collection, error) {
	if err := s.requireRepository(); err != nil {
		return Collection{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Collection{}, err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return Collection{}, err
	}
	return s.repo.GetCollection(ctx, CollectionLookupInput{CollectionID: collectionID, ActorUserID: actor.ID})
}

func (s *Service) UpdateCollection(ctx context.Context, collectionID string, input UpdateCollectionInput) (Collection, error) {
	if err := s.requireRepository(); err != nil {
		return Collection{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Collection{}, err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return Collection{}, err
	}
	input, err = normalizeUpdateInput(input)
	if err != nil {
		return Collection{}, err
	}
	return s.repo.UpdateCollection(ctx, UpdateCollectionRepositoryInput{
		CollectionID: collectionID,
		ActorUserID:  actor.ID,
		Name:         input.Name,
		Description:  input.Description,
		Icon:         input.Icon,
		Color:        input.Color,
	})
}

func (s *Service) DeleteCollection(ctx context.Context, collectionID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return err
	}
	return s.repo.DeleteCollection(ctx, DeleteCollectionRepositoryInput{CollectionID: collectionID, ActorUserID: actor.ID})
}

func (s *Service) CreateDocument(ctx context.Context, collectionID string, input BindDocumentInput) (Document, error) {
	if err := s.requireRepository(); err != nil {
		return Document{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return Document{}, err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
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
	ids := make([]string, 3)
	for index := range ids {
		ids[index], err = s.newID()
		if err != nil {
			return Document{}, fmt.Errorf("generate document identity: %w", err)
		}
	}
	sum := sha256.Sum256([]byte(collectionID + "\n" + input.FileID))
	return s.repo.CreateDocument(ctx, CreateDocumentRepositoryInput{
		DocumentID: ids[0], VersionID: ids[1], JobID: ids[2], CollectionID: collectionID,
		ActorUserID: actor.ID, FileID: input.FileID, IdempotencyKey: input.IdempotencyKey,
		RequestHash: hex.EncodeToString(sum[:]), ParseProcessor: "mineru",
	})
}

func (s *Service) ListDocuments(ctx context.Context, collectionID string, input ListDocumentsInput) (ApiPage[Document], error) {
	if err := s.requireRepository(); err != nil {
		return ApiPage[Document]{}, err
	}
	if s.cursorCodec == nil {
		return ApiPage[Document]{}, ErrCursorCodecRequired
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ApiPage[Document]{}, err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return ApiPage[Document]{}, err
	}
	input, err = normalizeDocumentListInput(input)
	if err != nil {
		return ApiPage[Document]{}, err
	}
	after, err := s.decodeDocumentCursor(input.Cursor, actor.ID, collectionID)
	if err != nil {
		return ApiPage[Document]{}, err
	}
	result, err := s.repo.ListDocuments(ctx, ListDocumentsRepositoryInput{
		CollectionID: collectionID, ActorUserID: actor.ID, Limit: input.Limit, After: after,
	})
	if err != nil {
		return ApiPage[Document]{}, err
	}
	page := ApiPage[Document]{Items: result.Items}
	if result.HasMore && len(result.Items) > 0 {
		last := result.Items[len(result.Items)-1]
		page.NextCursor, err = s.cursorCodec.Encode(teams.Cursor{
			Resource: documentCursorResource, UserID: actor.ID,
			FilterDigest: documentFilterDigest(collectionID), Sort: documentCursorSort,
			Values: []string{last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID},
		})
		if err != nil {
			return ApiPage[Document]{}, fmt.Errorf("encode document cursor: %w", err)
		}
	}
	return page, nil
}

func (s *Service) GetDocument(ctx context.Context, documentID string) (Document, error) {
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
	return s.repo.GetDocument(ctx, DocumentLookupInput{DocumentID: documentID, ActorUserID: actor.ID})
}

func (s *Service) GetDocumentContent(ctx context.Context, documentID string) (DocumentContentMetadata, io.ReadCloser, error) {
	if err := s.requireRepository(); err != nil {
		return DocumentContentMetadata{}, nil, err
	}
	if s.objectStore == nil {
		return DocumentContentMetadata{}, nil, ErrStorageRequired
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return DocumentContentMetadata{}, nil, err
	}
	documentID, err = normalizeUUID(documentID, "document id")
	if err != nil {
		return DocumentContentMetadata{}, nil, err
	}
	metadata, err := s.repo.GetActiveDocumentContentMetadata(ctx, DocumentLookupInput{
		DocumentID: documentID, ActorUserID: actor.ID,
	})
	if err != nil {
		return DocumentContentMetadata{}, nil, err
	}
	reader, _, err := s.objectStore.Get(ctx, metadata.ObjectKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return DocumentContentMetadata{}, nil, ErrDocumentNotFound
	}
	if err != nil {
		return DocumentContentMetadata{}, nil, fmt.Errorf("read active document content: %w", err)
	}
	return metadata, reader, nil
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func requireActor(ctx context.Context) (auth.User, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok || !isUUID(strings.TrimSpace(user.ID)) {
		return auth.User{}, ErrUnauthenticated
	}
	user.ID = strings.TrimSpace(user.ID)
	return user, nil
}

func normalizeCreateInput(input CreateCollectionInput) (CreateCollectionInput, error) {
	var err error
	input.Name, err = normalizeName(input.Name)
	if err != nil {
		return input, err
	}
	input.Description, err = normalizeDescription(input.Description)
	if err != nil {
		return input, err
	}
	input.Icon = strings.TrimSpace(input.Icon)
	if input.Icon == "" {
		input.Icon = "Folder"
	}
	if _, ok := validIcons[input.Icon]; !ok {
		return input, invalidCollectionPayload("icon is unsupported")
	}
	input.Color = strings.ToLower(strings.TrimSpace(input.Color))
	if input.Color == "" {
		input.Color = "blue"
	}
	if _, ok := validColors[input.Color]; !ok {
		return input, invalidCollectionPayload("color is unsupported")
	}
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	switch input.Scope {
	case ScopePersonal:
		if strings.TrimSpace(input.TeamID) != "" {
			return input, invalidCollectionPayload("teamId is forbidden for personal scope")
		}
		input.TeamID = ""
	case ScopeTeam:
		input.TeamID, err = normalizeUUID(input.TeamID, "teamId")
		if err != nil {
			return input, err
		}
	default:
		return input, invalidCollectionPayload("scope must be personal or team")
	}
	input.IdempotencyKey, err = normalizeIdempotencyKey(input.IdempotencyKey)
	return input, err
}

func normalizeUpdateInput(input UpdateCollectionInput) (UpdateCollectionInput, error) {
	if input.Name == nil && input.Description == nil && input.Icon == nil && input.Color == nil {
		return input, invalidCollectionPayload("at least one mutable field is required")
	}
	if input.Name != nil {
		value, normalizeErr := normalizeName(*input.Name)
		if normalizeErr != nil {
			return input, normalizeErr
		}
		input.Name = &value
	}
	if input.Description != nil {
		value, normalizeErr := normalizeDescription(*input.Description)
		if normalizeErr != nil {
			return input, normalizeErr
		}
		input.Description = &value
	}
	if input.Icon != nil {
		value := strings.TrimSpace(*input.Icon)
		if _, ok := validIcons[value]; !ok {
			return input, invalidCollectionPayload("icon is unsupported")
		}
		input.Icon = &value
	}
	if input.Color != nil {
		value := strings.ToLower(strings.TrimSpace(*input.Color))
		if _, ok := validColors[value]; !ok {
			return input, invalidCollectionPayload("color is unsupported")
		}
		input.Color = &value
	}
	return input, nil
}

func normalizeListInput(input ListCollectionsInput) (ListCollectionsInput, error) {
	input.Scope = strings.ToLower(strings.TrimSpace(input.Scope))
	input.TeamID = strings.TrimSpace(input.TeamID)
	if input.Scope != "" && input.Scope != ScopePersonal && input.Scope != ScopeTeam {
		return input, invalidCollectionPayload("scope must be personal or team")
	}
	if input.TeamID != "" {
		if input.Scope != ScopeTeam {
			return input, invalidCollectionPayload("teamId requires team scope")
		}
		var err error
		input.TeamID, err = normalizeUUID(input.TeamID, "teamId")
		if err != nil {
			return input, err
		}
	}
	if input.Scope == ScopePersonal && input.TeamID != "" {
		return input, invalidCollectionPayload("teamId is forbidden for personal scope")
	}
	if input.Limit == 0 {
		input.Limit = defaultPageLimit
	}
	if input.Limit < 1 || input.Limit > maximumPageLimit {
		return input, invalidCollectionPayload("limit must be between 1 and 100")
	}
	input.Cursor = strings.TrimSpace(input.Cursor)
	return input, nil
}

func normalizeDocumentListInput(input ListDocumentsInput) (ListDocumentsInput, error) {
	if input.Limit == 0 {
		input.Limit = defaultPageLimit
	}
	if input.Limit < 1 || input.Limit > maximumPageLimit {
		return input, invalidCollectionPayload("limit must be between 1 and 100")
	}
	input.Cursor = strings.TrimSpace(input.Cursor)
	return input, nil
}

func normalizeName(value string) (string, error) {
	if !utf8.ValidString(value) || containsControlOrFormat(value) {
		return "", invalidCollectionPayload("name contains invalid characters")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalidCollectionPayload("name is required")
	}
	if len(value) > maximumCollectionNameBytes || utf8.RuneCountInString(value) > maximumCollectionNameRunes {
		return "", invalidCollectionPayload("name is too long")
	}
	return value, nil
}

func normalizeDescription(value string) (string, error) {
	if !utf8.ValidString(value) || containsControlOrFormat(value) {
		return "", invalidCollectionPayload("description contains invalid characters")
	}
	value = strings.TrimSpace(value)
	if len(value) > maximumCollectionDescriptionBytes || utf8.RuneCountInString(value) > maximumCollectionDescriptionRunes {
		return "", invalidCollectionPayload("description is too long")
	}
	return value, nil
}

func normalizeIdempotencyKey(value string) (string, error) {
	if !utf8.ValidString(value) || containsControlOrFormat(value) {
		return "", invalidCollectionPayload("idempotencyKey contains invalid characters")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalidCollectionPayload("idempotencyKey is required")
	}
	if len(value) > maximumIdempotencyBytes {
		return "", invalidCollectionPayload("idempotencyKey is too long")
	}
	return value, nil
}

func normalizeUUID(value string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if !isUUID(value) {
		return "", invalidCollectionPayload(label + " must be a UUID")
	}
	return value, nil
}

func containsControlOrFormat(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return true
		}
	}
	return false
}

func hashCreateInput(input CreateCollectionInput) (string, error) {
	payload, err := json.Marshal(struct {
		Name, Description, Icon, Color, Scope, TeamID string
	}{input.Name, input.Description, input.Icon, input.Color, input.Scope, input.TeamID})
	if err != nil {
		return "", fmt.Errorf("marshal canonical collection request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func collectionFilterDigest(scope, teamID string) string {
	return teams.CursorFilterDigest("scope=" + scope + "\nteamId=" + teamID)
}

func documentFilterDigest(collectionID string) string {
	return teams.CursorFilterDigest("collectionId=" + collectionID)
}

func (s *Service) decodeCursor(encoded, userID, scope, teamID string) (*CollectionPageCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	cursor, err := s.cursorCodec.Decode(encoded, teams.CursorContext{
		Resource: collectionCursorResource, UserID: userID,
		FilterDigest: collectionFilterDigest(scope, teamID), Sort: collectionCursorSort,
	})
	if err != nil || len(cursor.Values) != 2 {
		return nil, invalidCollectionPayload("cursor is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.Values[0])
	if err != nil {
		return nil, invalidCollectionPayload("cursor is invalid")
	}
	id, err := normalizeUUID(cursor.Values[1], "cursor id")
	if err != nil {
		return nil, invalidCollectionPayload("cursor is invalid")
	}
	return &CollectionPageCursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}

func (s *Service) decodeDocumentCursor(encoded, userID, collectionID string) (*DocumentPageCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	cursor, err := s.cursorCodec.Decode(encoded, teams.CursorContext{
		Resource: documentCursorResource, UserID: userID,
		FilterDigest: documentFilterDigest(collectionID), Sort: documentCursorSort,
	})
	if err != nil || len(cursor.Values) != 2 {
		return nil, invalidCollectionPayload("cursor is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.Values[0])
	if err != nil {
		return nil, invalidCollectionPayload("cursor is invalid")
	}
	id, err := normalizeUUID(cursor.Values[1], "cursor id")
	if err != nil {
		return nil, invalidCollectionPayload("cursor is invalid")
	}
	return &DocumentPageCursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}
