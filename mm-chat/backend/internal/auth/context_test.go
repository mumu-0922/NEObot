package auth

import (
	"context"
	"testing"
)

func TestUserContextRoundTripNormalizesRole(t *testing.T) {
	ctx := WithUser(context.Background(), User{
		ID:          " 11111111-1111-4111-8111-111111111111 ",
		DisplayName: " User One ",
	})

	user, ok := UserFromContext(ctx)
	if !ok {
		t.Fatal("UserFromContext() ok = false, want true")
	}
	if user.ID != "11111111-1111-4111-8111-111111111111" || user.DisplayName != "User One" || user.Role != "user" {
		t.Fatalf("UserFromContext() = %#v", user)
	}
}

func TestUserOrDevelopmentFallback(t *testing.T) {
	user := UserOrDevelopment(context.Background())
	if user.ID != DevelopmentUserID || user.DisplayName != DevelopmentDisplayName || user.Role != "user" {
		t.Fatalf("UserOrDevelopment() = %#v", user)
	}
}

func TestUserFromSession(t *testing.T) {
	user := UserFromSession(Session{
		UserID:      "22222222-2222-4222-8222-222222222222",
		DisplayName: "Session User",
	})
	if user.ID != "22222222-2222-4222-8222-222222222222" || user.DisplayName != "Session User" || user.Role != "user" {
		t.Fatalf("UserFromSession() = %#v", user)
	}
}
