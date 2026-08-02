package memoryjudge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestFailureTaxonomyBindsEveryProviderAndJudgeLocalCategory(t *testing.T) {
	categories := FailureCategories()
	if len(categories) != 24 {
		t.Fatalf("category count=%d categories=%v", len(categories), categories)
	}
	seen := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		if !ValidFailureCategory(category) {
			t.Fatalf("invalid category %q", category)
		}
		if _, duplicate := seen[category]; duplicate {
			t.Fatalf("duplicate category %q", category)
		}
		seen[category] = struct{}{}
	}
	for _, category := range chat.ProviderFailureCategories() {
		if _, ok := seen[string(category)]; !ok {
			t.Fatalf("Provider category %q missing", category)
		}
	}
	body, err := json.Marshal(categories)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	if got := hex.EncodeToString(digest[:]); got != FailureTaxonomySHA256 {
		t.Fatalf("taxonomy SHA-256=%s", got)
	}
}

func TestFailureCategoryUsesTypedCausesAndFixedUnknownFallback(t *testing.T) {
	if got := FailureCategory(context.DeadlineExceeded); got != string(chat.ProviderFailureContextDeadline) {
		t.Fatalf("deadline=%q", got)
	}
	if got := FailureCategory(context.Canceled); got != string(chat.ProviderFailureContextCanceled) {
		t.Fatalf("canceled=%q", got)
	}
	_, outputErr := usermemory.DecodeHybridCandidateJudgeOutput(
		[]byte(`{"schemaVersion":"drifted","selectedOrdinals":[]}`),
		1,
	)
	if got := FailureCategory(outputErr); got != FailureOutputSchemaInvalid {
		t.Fatalf("schema=%q", got)
	}
	private := errors.New("private Provider response and Memory body")
	if got := FailureCategory(private); got != FailureUnclassified {
		t.Fatalf("unknown=%q", got)
	}
	wrapped := NewFailure(FailureProvenanceDrift, private)
	if got := FailureCategory(wrapped); got != FailureProvenanceDrift ||
		!errors.Is(wrapped, private) {
		t.Fatalf("wrapped=%q/%v", got, wrapped)
	}
	if FailureCategory(nil) != "" || ValidFailureCategory("private") ||
		AttemptFailureCategory(FailureProvenanceDrift) ||
		AttemptFailureCategory(FailureRecorderStateConflict) ||
		!AttemptFailureCategory(FailureOutputJSONInvalid) {
		t.Fatal("failure classification contract drifted")
	}
}
