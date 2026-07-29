package memoryauthor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	testReviewerID = "11111111-1111-4111-8111-111111111111"
	testHoldoutID  = "22222222-2222-4222-8222-222222222222"
)

func newTestPoolRoot(t *testing.T) string {
	t.Helper()
	pool, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "v1")
	if err := PublishPool(root, pool); err != nil {
		t.Fatal(err)
	}
	return root
}

// seedTestOnlyDecisions creates explicit test-fixture events without invoking
// the operator UI. These events exist only in t.TempDir and are never human
// review evidence.
func seedTestOnlyDecisions(t *testing.T, root string) {
	t.Helper()
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := Diagnose(state.FixtureManifest, state.Golden)
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(map[string]struct{}, len(diagnostic.WitnessCaseIDs))
	for _, caseID := range diagnostic.WitnessCaseIDs {
		accepted[caseID] = struct{}{}
	}
	previousHash := state.LastEventSHA256
	eventsDirectory := filepath.Join(root, ReviewDirectory, ReviewEventsDirectory)
	reviewedAt := time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC).Format(time.RFC3339)
	for index, candidate := range state.Cases {
		action := ReviewActionReject
		if _, ok := accepted[candidate.Snapshot.Case.ID]; ok {
			action = ReviewActionAccept
		}
		fixtureHash, err := fixtureSHA256(candidate.Snapshot.Fixture)
		if err != nil {
			t.Fatal(err)
		}
		event := ReviewEvent{
			SchemaVersion: ReviewEventVersion,
			Sequence:      uint64(index + 1), PreviousEventSHA256: previousHash,
			Action: action, CaseID: candidate.Snapshot.Case.ID,
			BeforeContentSHA256:  candidate.ContentSHA256,
			AfterContentSHA256:   candidate.ContentSHA256,
			FixtureContentSHA256: fixtureHash,
			ReviewerID:           testReviewerID, OccurredAt: reviewedAt,
		}
		body, err := marshalCanonical(event)
		if err != nil {
			t.Fatal(err)
		}
		eventHash := sha256Hex(body)
		path := filepath.Join(eventsDirectory, eventFileName(event.Sequence, eventHash))
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		previousHash = eventHash
	}
}

func eventFileName(sequence uint64, hash string) string {
	return fmt.Sprintf("%08d-%s.json", sequence, hash)
}
