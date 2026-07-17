package knowledge

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestCollectionQueryTimeConsentsDoNotInvalidateSearchProjection(t *testing.T) {
	for _, purposes := range [][]string{{"answer"}, {"rerank"}, {"answer", "rerank"}} {
		if collectionConsentAffectsProjection(purposes) {
			t.Fatalf("query-time purposes %v unexpectedly affect the search projection", purposes)
		}
	}
	for _, purposes := range [][]string{{"parse"}, {"passage_embedding"}, {"parse", "rerank"}} {
		if !collectionConsentAffectsProjection(purposes) {
			t.Fatalf("purposes %v must affect the search projection", purposes)
		}
	}
}

func TestCollectionConsentServiceNormalizesAndBindsActor(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{consents: []ProcessingConsent{{Processor: "mineru", Decision: "granted", DecidedAt: now}}}
	service := NewService(repo)
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	_, err := service.PutCollectionConsent(ctx, "22222222-2222-4222-8222-222222222222", " mineru ", PutConsentInput{
		Purposes: []string{"rerank", "parse", "parse"}, DataTypes: []string{"text/plain", "application/pdf"},
		PolicyVersion: "v1", ExpiresAt: now.Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	input := repo.putConsent
	if input.ActorUserID != testActorID || input.Processor != "mineru" || len(input.Purposes) != 2 || input.Purposes[0] != "parse" || input.DataTypes[0] != "application/pdf" {
		t.Fatalf("normalized consent input = %#v", input)
	}
	if err := service.RevokeCollectionConsent(ctx, input.CollectionID, "mineru"); err != nil {
		t.Fatal(err)
	}
	if repo.revokedConsent.ActorUserID != testActorID {
		t.Fatalf("revoke actor = %#v", repo.revokedConsent)
	}
}

func TestCollectionConsentServiceRejectsInvalidTerms(t *testing.T) {
	service := NewService(&fakeRepository{})
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	base := PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"application/pdf"}, PolicyVersion: "v1"}
	for name, mutate := range map[string]func(*PutConsentInput){
		"purpose": func(v *PutConsentInput) { v.Purposes = []string{"query_embedding"} },
		"mime":    func(v *PutConsentInput) { v.DataTypes = []string{"application/*"} },
		"policy":  func(v *PutConsentInput) { v.PolicyVersion = "secret" },
		"expiry":  func(v *PutConsentInput) { v.ExpiresAt = "tomorrow" },
	} {
		t.Run(name, func(t *testing.T) {
			value := base
			mutate(&value)
			if _, err := service.PutCollectionConsent(ctx, "22222222-2222-4222-8222-222222222222", "mineru", value); err == nil {
				t.Fatal("error = nil")
			}
		})
	}
}

func TestConsentDataTypesCannotWidenGovernanceWildcard(t *testing.T) {
	if isDataTypeSubset([]string{"*"}, []string{"application/pdf"}) {
		t.Fatal("global consent wildcard widened exact governance data types")
	}
	if !isDataTypeSubset([]string{"application/pdf"}, []string{"*"}) {
		t.Fatal("global governance wildcard did not allow exact consent data type")
	}
}

func TestQueryConsentServiceAllowsOnlyCurrentActorAndQueryPurposes(t *testing.T) {
	repo := &fakeRepository{queryConsents: []ProcessingConsent{{Processor: "jina", Decision: "granted"}}}
	service := NewService(repo)
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	_, err := service.PutQueryConsent(ctx, " jina ", PutConsentInput{
		Purposes: []string{"rerank", "query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.putQueryConsent.ActorUserID != testActorID || repo.putQueryConsent.Purposes[0] != "query_embedding" {
		t.Fatalf("query consent input = %#v", repo.putQueryConsent)
	}
	if err := service.RevokeQueryConsent(ctx, "jina"); err != nil {
		t.Fatal(err)
	}
	if repo.revokedQuery.ActorUserID != testActorID {
		t.Fatalf("query revoke = %#v", repo.revokedQuery)
	}
	if _, err := service.PutQueryConsent(ctx, "jina", PutConsentInput{Purposes: []string{"parse"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1"}); err == nil {
		t.Fatal("query consent accepted collection-only purpose")
	}
}

func TestConsentServiceBindsExactEndpointModelTogether(t *testing.T) {
	repo := &fakeRepository{queryConsents: []ProcessingConsent{{Processor: "jina"}}}
	service := NewService(repo)
	ctx := auth.WithUser(context.Background(), auth.User{ID: testActorID})
	input := PutConsentInput{Purposes: []string{"query_embedding"}, DataTypes: []string{"text/plain"}, PolicyVersion: "v1"}
	identity := ProcessorModelIdentity{Processor: " jina ", EndpointID: " hosted ", ModelID: " embed-v4 "}
	if _, err := service.PutQueryConsentForModel(ctx, identity, input); err != nil {
		t.Fatal(err)
	}
	if repo.putQueryConsent.Processor != "jina" || repo.putQueryConsent.EndpointID != "hosted" ||
		repo.putQueryConsent.ModelID != "embed-v4" {
		t.Fatalf("exact query identity = %#v", repo.putQueryConsent)
	}
	if _, err := service.PutQueryConsentForModel(ctx, ProcessorModelIdentity{
		Processor: "jina", EndpointID: "hosted",
	}, input); err == nil {
		t.Fatal("partial endpoint/model identity accepted")
	}
}
