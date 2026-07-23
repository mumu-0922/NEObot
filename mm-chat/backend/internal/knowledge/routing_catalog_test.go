package knowledge

import (
	"context"
	"slices"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestRoutingCatalogTermsCoverCJKBigramAndEnglishLexicalSignals(t *testing.T) {
	cjk := routingCatalogTerms("有小作文模板嘛")
	for _, want := range []string{"作文", "文模", "模板"} {
		if !slices.Contains(cjk, want) {
			t.Fatalf("CJK terms %#v do not contain %q", cjk, want)
		}
	}
	english := routingCatalogTerms("Linux Registration Template guide")
	for _, want := range []string{"linux", "registration", "template", "guide"} {
		if !slices.Contains(english, want) {
			t.Fatalf("English terms %#v do not contain %q", english, want)
		}
	}
}

func TestBuildRoutingCatalogSelectsFiveRelevantAndThreeRepresentative(t *testing.T) {
	documents := make([]RoutingCatalogDocument, 0, 12)
	for index := range 7 {
		documents = append(documents, RoutingCatalogDocument{
			Title: "relevant-" + string(rune('a'+index)), RelevanceScore: 10 - index,
		})
	}
	for index := range 5 {
		documents = append(documents, RoutingCatalogDocument{
			Title: "representative-" + string(rune('a'+index)), Representative: true,
			UpdatedAt: time.Now().Add(-time.Duration(index) * time.Hour),
		})
	}
	repo := &fakeRepository{routingCatalog: []RoutingCatalogCollection{{
		ID: "22222222-2222-4222-8222-222222222222", Name: "Templates",
		ActiveDocumentCount: len(documents), Documents: documents,
	}}}
	service := NewService(repo)
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	catalog, err := service.BuildRoutingCatalog(ctx, RoutingCatalogInput{
		CollectionIDs: []string{"22222222-2222-4222-8222-222222222222"},
		QueryText:     " registration   template ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.HasLexicalMatch || len(catalog.Collections) != 1 ||
		len(catalog.Collections[0].Documents) != 8 {
		t.Fatalf("catalog = %#v", catalog)
	}
	for index, document := range catalog.Collections[0].Documents {
		if index < 5 && document.RelevanceScore <= 0 {
			t.Fatalf("relevant document %d = %#v", index, document)
		}
		if index >= 5 && !document.Representative {
			t.Fatalf("representative document %d = %#v", index, document)
		}
	}
	if repo.routingCatalogIn.ActorUserID != testActorID ||
		repo.routingCatalogIn.QueryText != "registration template" ||
		!slices.Contains(repo.routingCatalogIn.QueryTerms, "template") {
		t.Fatalf("repository input = %#v", repo.routingCatalogIn)
	}
}

func TestBuildRoutingCatalogRepresentativeFallbackDoesNotBecomeStrongMatch(t *testing.T) {
	repo := &fakeRepository{routingCatalog: []RoutingCatalogCollection{{
		ID: "22222222-2222-4222-8222-222222222222", Name: "General",
		ActiveDocumentCount: 1,
		Documents: []RoutingCatalogDocument{{
			Title: "unrelated.txt", Representative: true, RelevanceScore: 0,
		}},
	}}}
	service := NewService(repo)
	catalog, err := service.BuildRoutingCatalog(
		auth.WithUser(context.Background(), auth.User{ID: testActorID}),
		RoutingCatalogInput{
			CollectionIDs: []string{"22222222-2222-4222-8222-222222222222"},
			QueryText:     "birthday greeting",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.HasLexicalMatch || len(catalog.Collections[0].Documents) != 1 {
		t.Fatalf("fallback catalog = %#v", catalog)
	}
}

func TestBuildRoutingCatalogRejectsMoreThanEightCollections(t *testing.T) {
	ids := make([]string, 9)
	for index := range ids {
		ids[index] = "22222222-2222-4222-8222-22222222222" + string(rune('0'+index))
	}
	_, err := NewService(&fakeRepository{}).BuildRoutingCatalog(
		auth.WithUser(context.Background(), auth.User{ID: testActorID}),
		RoutingCatalogInput{CollectionIDs: ids, QueryText: "fixture"},
	)
	if err == nil {
		t.Fatal("nine selected collections were accepted")
	}
}
