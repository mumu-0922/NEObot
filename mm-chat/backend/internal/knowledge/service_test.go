package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/storage"
	"neo-chat/mm-chat/backend/internal/teams"
)

const testActorID = "11111111-1111-4111-8111-111111111111"

func TestServiceNormalizesCreateAndBindsActor(t *testing.T) {
	repo := &fakeRepository{createResult: testCollection("22222222-2222-4222-8222-222222222222")}
	service := NewService(repo, WithIDGenerator(func() (string, error) {
		return repo.createResult.ID, nil
	}))
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})

	got, err := service.CreateCollection(ctx, CreateCollectionInput{
		Name: " Research ", Scope: "PERSONAL", IdempotencyKey: " create-1 ",
	})
	if err != nil {
		t.Fatalf("CreateCollection() error = %v", err)
	}
	if got.ID != repo.createResult.ID {
		t.Fatalf("CreateCollection() id = %q", got.ID)
	}
	input := repo.created
	if input.ActorUserID != testActorID || input.Name != "Research" || input.Scope != ScopePersonal {
		t.Fatalf("repository input = %#v", input)
	}
	if input.Icon != "Folder" || input.Color != "blue" || input.IdempotencyKey != "create-1" {
		t.Fatalf("repository defaults = %#v", input)
	}
	if len(input.CreateRequestHash) != 64 {
		t.Fatalf("request hash length = %d", len(input.CreateRequestHash))
	}
}

func TestServiceRejectsScopeAndImmutableIdentityInputs(t *testing.T) {
	service := NewService(&fakeRepository{})
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	for _, input := range []CreateCollectionInput{
		{Name: "A", Scope: ScopePersonal, TeamID: "33333333-3333-4333-8333-333333333333", IdempotencyKey: "1"},
		{Name: "A", Scope: ScopeTeam, IdempotencyKey: "1"},
		{Name: "A", Scope: "shared", IdempotencyKey: "1"},
	} {
		if _, err := service.CreateCollection(ctx, input); err == nil {
			t.Fatalf("CreateCollection(%#v) error = nil", input)
		}
	}
}

func TestServiceCollectionCursorIsUserAndFilterBound(t *testing.T) {
	codec, err := teams.NewCursorCodec(teams.CursorKeyring{
		ActiveKeyID: "test", Keys: map[string][]byte{"test": []byte("01234567890123456789012345678901")},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{listResult: CollectionPageResult{
		Items: []Collection{testCollection("22222222-2222-4222-8222-222222222222")}, HasMore: true,
	}}
	service := NewService(repo, WithCursorCodec(codec))
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	page, err := service.ListCollections(ctx, ListCollectionsInput{Scope: ScopePersonal, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor == "" {
		t.Fatal("next cursor is empty")
	}
	if _, err := service.ListCollections(ctx, ListCollectionsInput{Scope: ScopeTeam, Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor replay across filters succeeded")
	}
	other := auth.WithUser(context.Background(), auth.User{ID: "44444444-4444-4444-8444-444444444444"})
	if _, err := service.ListCollections(other, ListCollectionsInput{Scope: ScopePersonal, Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor replay across users succeeded")
	}
}

func TestServiceDocumentCursorAndContentAuthorizationOrder(t *testing.T) {
	codec, err := teams.NewCursorCodec(teams.CursorKeyring{
		ActiveKeyID: "test", Keys: map[string][]byte{"test": []byte("01234567890123456789012345678901")},
	})
	if err != nil {
		t.Fatal(err)
	}
	document := Document{ID: "22222222-2222-4222-8222-222222222222",
		CollectionID: "33333333-3333-4333-8333-333333333333", Status: "active",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	store := &fakeObjectStore{body: []byte("source")}
	repo := &fakeRepository{documentPage: DocumentPageResult{Items: []Document{document}, HasMore: true},
		documentResult: document, contentResult: DocumentContentMetadata{
			DocumentID: document.ID, ObjectKey: "private/source", FileName: "source.txt",
			MIMEType: "text/plain", ByteSize: 6,
		}}
	service := NewService(repo, WithCursorCodec(codec), WithObjectStore(store))
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	page, err := service.ListDocuments(ctx, document.CollectionID, ListDocumentsInput{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("ListDocuments() = %#v, %v", page, err)
	}
	if _, err := service.ListDocuments(ctx, "44444444-4444-4444-8444-444444444444",
		ListDocumentsInput{Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("document cursor replay across collections succeeded")
	}

	metadata, reader, err := service.GetDocumentContent(ctx, document.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if metadata.ObjectKey != "private/source" || store.gets != 1 {
		t.Fatalf("content metadata/store gets = %#v/%d", metadata, store.gets)
	}

	store.err = storage.ErrObjectNotFound
	if _, _, err := service.GetDocumentContent(ctx, document.ID); err != ErrDocumentNotFound {
		t.Fatalf("missing object error = %v", err)
	}
	store.err = nil
	repo.err = ErrDocumentNotFound
	if _, _, err := service.GetDocumentContent(ctx, document.ID); err != ErrDocumentNotFound {
		t.Fatalf("hidden content error = %v", err)
	}
	if store.gets != 2 {
		t.Fatalf("object store called before authorization: %d", store.gets)
	}
}

func TestServiceCreatesServerSelectedReplacementVersion(t *testing.T) {
	repo := &fakeRepository{documentResult: Document{ID: "22222222-2222-4222-8222-222222222222"}}
	ids := []string{"33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"}
	service := NewService(repo, WithIDGenerator(func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}))
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	_, err := service.CreateDocumentVersion(ctx, repo.documentResult.ID, BindDocumentInput{
		FileID: "55555555-5555-4555-8555-555555555555", IdempotencyKey: " replace-1 ",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := repo.versionCreated
	expectedHash := sha256.Sum256([]byte(repo.documentResult.ID + "\n55555555-5555-4555-8555-555555555555"))
	if input.ActorUserID != testActorID || input.VersionID != "33333333-3333-4333-8333-333333333333" ||
		input.JobID != "44444444-4444-4444-8444-444444444444" || input.ParseProcessor != "mineru" ||
		input.IdempotencyKey != "replace-1" || input.RequestHash != hex.EncodeToString(expectedHash[:]) {
		t.Fatalf("replacement repository input = %#v", input)
	}
}

type fakeRepository struct {
	created        CreateCollectionRepositoryInput
	createResult   Collection
	listResult     CollectionPageResult
	documentResult Document
	documentPage   DocumentPageResult
	contentResult  DocumentContentMetadata
	versionCreated CreateDocumentVersionRepositoryInput
	err            error
}

type fakeObjectStore struct {
	body []byte
	gets int
	err  error
}

func (store *fakeObjectStore) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}
func (store *fakeObjectStore) Get(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	store.gets++
	if store.err != nil {
		return nil, storage.ObjectInfo{}, store.err
	}
	return io.NopCloser(bytes.NewReader(store.body)), storage.ObjectInfo{Key: key, Size: int64(len(store.body))}, nil
}
func (store *fakeObjectStore) Delete(context.Context, string) error { return nil }

func (repo *fakeRepository) CreateCollection(_ context.Context, input CreateCollectionRepositoryInput) (Collection, error) {
	repo.created = input
	return repo.createResult, repo.err
}
func (repo *fakeRepository) ListCollections(context.Context, ListCollectionsRepositoryInput) (CollectionPageResult, error) {
	return repo.listResult, repo.err
}
func (repo *fakeRepository) GetCollection(context.Context, CollectionLookupInput) (Collection, error) {
	return repo.createResult, repo.err
}
func (repo *fakeRepository) UpdateCollection(context.Context, UpdateCollectionRepositoryInput) (Collection, error) {
	return repo.createResult, repo.err
}
func (repo *fakeRepository) DeleteCollection(context.Context, DeleteCollectionRepositoryInput) error {
	return repo.err
}
func (repo *fakeRepository) CreateDocument(context.Context, CreateDocumentRepositoryInput) (Document, error) {
	return repo.documentResult, repo.err
}
func (repo *fakeRepository) CreateDocumentVersion(_ context.Context, input CreateDocumentVersionRepositoryInput) (Document, error) {
	repo.versionCreated = input
	return repo.documentResult, repo.err
}
func (repo *fakeRepository) ListDocuments(context.Context, ListDocumentsRepositoryInput) (DocumentPageResult, error) {
	return repo.documentPage, repo.err
}
func (repo *fakeRepository) GetDocument(context.Context, DocumentLookupInput) (Document, error) {
	return repo.documentResult, repo.err
}
func (repo *fakeRepository) GetActiveDocumentContentMetadata(context.Context, DocumentLookupInput) (DocumentContentMetadata, error) {
	return repo.contentResult, repo.err
}

func testCollection(id string) Collection {
	return Collection{ID: id, Name: "Research", Description: "", Icon: "Folder", Color: "blue",
		Scope: ScopePersonal, Permissions: Permissions{Read: true, Manage: true, ManageConsent: true},
		ACLRevision: 1, VisibilityEpoch: 1, CollectionProcessingRevision: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}
