package knowledge

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
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

type fakeRepository struct {
	created      CreateCollectionRepositoryInput
	createResult Collection
	listResult   CollectionPageResult
	err          error
}

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
	return Document{}, repo.err
}

func testCollection(id string) Collection {
	return Collection{ID: id, Name: "Research", Description: "", Icon: "Folder", Color: "blue",
		Scope: ScopePersonal, Permissions: Permissions{Read: true, Manage: true, ManageConsent: true},
		ACLRevision: 1, VisibilityEpoch: 1, CollectionProcessingRevision: 1,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
}
