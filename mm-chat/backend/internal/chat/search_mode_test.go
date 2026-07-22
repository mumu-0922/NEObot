package chat

import (
	"context"
	"errors"
	"testing"
)

func TestSearchModeFromConfigPrefersThreeStateModeAndMapsLegacyToggle(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   chatSearchMode
	}{
		{name: "off wins legacy true", config: map[string]any{"searchMode": "off", "useSearch": true}, want: chatSearchModeOff},
		{name: "built in", config: map[string]any{"searchMode": "model_builtin"}, want: chatSearchModeModelBuiltIn},
		{name: "external", config: map[string]any{"searchMode": "external"}, want: chatSearchModeExternal},
		{name: "legacy true", config: map[string]any{"useSearch": true}, want: chatSearchModeExternal},
		{name: "legacy false", config: map[string]any{"useSearch": false}, want: chatSearchModeOff},
		{name: "invalid", config: map[string]any{"searchMode": "invalid"}, want: chatSearchModeOff},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := searchModeFromConfig(test.config); got != test.want {
				t.Fatalf("searchModeFromConfig() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizeConversationSearchMetadataAndInheritance(t *testing.T) {
	fallback := chatSearchModeModelBuiltIn
	metadata := map[string]any{}
	if err := normalizeConversationSearchMetadata(metadata, &fallback); err != nil {
		t.Fatal(err)
	}
	if metadata["searchMode"] != "model_builtin" || metadata["useSearch"] != true {
		t.Fatalf("fallback metadata = %#v", metadata)
	}

	legacy := map[string]any{"useSearch": true}
	if err := normalizeConversationSearchMetadata(legacy, nil); err != nil {
		t.Fatal(err)
	}
	if legacy["searchMode"] != "external" || legacy["useSearch"] != true {
		t.Fatalf("legacy metadata = %#v", legacy)
	}

	if err := normalizeConversationSearchMetadata(
		map[string]any{"searchMode": "invalid"}, nil,
	); err == nil {
		t.Fatal("invalid search mode was accepted")
	}

	got := inheritedConversationSearchMode([]Conversation{
		{Metadata: map[string]any{"searchMode": "model_builtin"}},
		{Metadata: map[string]any{"searchMode": "external"}},
	})
	if got != chatSearchModeModelBuiltIn {
		t.Fatalf("inherited mode = %q", got)
	}
}

func TestCreateConversationInheritsLatestSearchModeAndNormalizesLegacy(t *testing.T) {
	repo := newFakeRepository()
	latest := fakeConversation(testConversationID, "Latest", 0)
	latest.Metadata = map[string]any{"searchMode": "model_builtin", "useSearch": true}
	repo.conversations = []Conversation{latest}

	created, err := NewService(repo).CreateConversation(
		context.Background(),
		CreateConversationInput{Title: "Inherited"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Metadata["searchMode"] != "model_builtin" ||
		created.Metadata["useSearch"] != true {
		t.Fatalf("inherited metadata = %#v", created.Metadata)
	}

	legacy, err := NewService(repo).CreateConversation(
		context.Background(),
		CreateConversationInput{
			Title:    "Legacy",
			Metadata: map[string]any{"useSearch": true},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Metadata["searchMode"] != "external" ||
		legacy.Metadata["useSearch"] != true {
		t.Fatalf("legacy metadata = %#v", legacy.Metadata)
	}

	_, err = NewService(repo).CreateConversation(
		context.Background(),
		CreateConversationInput{
			Title:    "Invalid",
			Metadata: map[string]any{"searchMode": "automatic"},
		},
	)
	var validation ValidationError
	if err == nil || !errors.As(err, &validation) || validation.Code != "INVALID_SEARCH_MODE" {
		t.Fatalf("invalid mode error = %#v", err)
	}
}
