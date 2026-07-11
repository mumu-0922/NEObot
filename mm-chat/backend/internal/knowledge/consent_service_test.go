package knowledge

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

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
