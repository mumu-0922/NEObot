package memoryauthor

import (
	"bytes"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestGenerateIsDeterministicAndMeetsCandidateProfile(t *testing.T) {
	first, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.FixtureJSON, second.FixtureJSON) ||
		!bytes.Equal(first.GoldenJSON, second.GoldenJSON) ||
		!bytes.Equal(first.ManifestJSON, second.ManifestJSON) {
		t.Fatal("Generate() is not byte deterministic")
	}
	if len(first.Golden.Cases) != 650 || len(first.FixtureManifest.Fixtures) != 650 {
		t.Fatalf("generated cases/fixtures = %d/%d", len(first.Golden.Cases), len(first.FixtureManifest.Fixtures))
	}
	if first.Manifest.SplitCounts != (CountBySplit{Development: 390, Validation: 130, Holdout: 130}) ||
		first.Manifest.LanguageCounts != (CountByLanguage{Chinese: 455, Mixed: 130, English: 65}) {
		t.Fatalf("generated profile counts = %+v %+v", first.Manifest.SplitCounts, first.Manifest.LanguageCounts)
	}
	for _, count := range first.Manifest.SliceCounts {
		if count.Total < 65 || count.Development < 39 || count.Validation < 13 || count.Holdout < 13 {
			t.Fatalf("slice count = %+v", count)
		}
	}
}

func TestScopeHierarchyAllowsGlobalProjectAndCurrentConversationOnly(t *testing.T) {
	conversation := memoryScope("user-a", "project-a", "conversation-a")
	for _, allowed := range []struct {
		name  string
		scope memoryeval.Scope
	}{
		{name: "global", scope: memoryScope("user-a", "", "")},
		{name: "project", scope: memoryScope("user-a", "project-a", "")},
		{name: "conversation", scope: memoryScope("user-a", "project-a", "conversation-a")},
	} {
		if !scopeContains(conversation, allowed.scope) {
			t.Fatalf("conversation did not authorize %s scope", allowed.name)
		}
	}
	for _, denied := range []memoryeval.Scope{
		memoryScope("user-b", "", ""),
		memoryScope("user-a", "project-b", ""),
		memoryScope("user-a", "project-a", "conversation-b"),
	} {
		if scopeContains(conversation, denied) {
			t.Fatalf("conversation authorized wrong scope: %+v", denied)
		}
	}
}

func memoryScope(user, project, conversation string) memoryeval.Scope {
	return memoryeval.Scope{
		UserAlias: user, ProjectAlias: project, ConversationAlias: conversation,
	}
}
