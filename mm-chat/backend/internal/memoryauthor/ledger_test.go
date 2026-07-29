package memoryauthor

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestReviewLedgerResumeAndEditInvalidatesPriorDecision(t *testing.T) {
	root := newTestPoolRoot(t)
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	first := state.Cases[0]
	acceptedAt := time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC)
	result, err := ApplyReview(root, ReviewInput{
		Action: ReviewActionAccept, CaseID: first.Snapshot.Case.ID,
		ExpectedSequence: state.LastSequence, ExpectedContentSHA256: first.ContentSHA256,
		ReviewerID: testReviewerID, Clock: func() time.Time { return acceptedAt },
	})
	if err != nil || result.Decision != DecisionAccepted || result.Sequence != 1 {
		t.Fatalf("ApplyReview(accept) = %+v, %v", result, err)
	}
	resumed, err := LoadReviewState(root)
	if err != nil || resumed.Cases[0].Decision != DecisionAccepted {
		t.Fatalf("resumed state = %+v, %v", resumed.Cases[0], err)
	}
	if _, err := ApplyReview(root, ReviewInput{
		Action: ReviewActionReject, CaseID: resumed.Cases[0].Snapshot.Case.ID,
		ExpectedSequence:      resumed.LastSequence,
		ExpectedContentSHA256: resumed.Cases[0].ContentSHA256,
		ReviewerID:            testReviewerID,
		Clock:                 func() time.Time { return acceptedAt.Add(-time.Minute) },
	}); err == nil || !strings.Contains(err.Error(), "clock moved backwards") {
		t.Fatalf("backwards review clock error = %v", err)
	}
	forged := resumed.Cases[0].Snapshot
	forged.Case.Query += " forged"
	forged.Case.Review = memoryeval.Review{
		State: "human_reviewed", ReviewerID: testReviewerID,
		ReviewedAt: acceptedAt.Format(time.RFC3339),
	}
	if _, err := ApplyReview(root, ReviewInput{
		Action: ReviewActionEdit, CaseID: forged.Case.ID,
		ExpectedSequence:      resumed.LastSequence,
		ExpectedContentSHA256: resumed.Cases[0].ContentSHA256,
		ReviewerID:            testReviewerID, Snapshot: &forged,
		Clock: func() time.Time { return acceptedAt.Add(time.Minute) },
	}); err == nil || !strings.Contains(err.Error(), "attestation") {
		t.Fatalf("forged edit attestation error = %v", err)
	}
	edited := resumed.Cases[0].Snapshot
	edited.Case.Query += "（人工修订）"
	result, err = ApplyReview(root, ReviewInput{
		Action: ReviewActionEdit, CaseID: edited.Case.ID,
		ExpectedSequence:      resumed.LastSequence,
		ExpectedContentSHA256: resumed.Cases[0].ContentSHA256,
		ReviewerID:            testReviewerID, Snapshot: &edited,
		Clock: func() time.Time { return acceptedAt.Add(time.Minute) },
	})
	if err != nil || result.Decision != DecisionPending || result.Sequence != 2 {
		t.Fatalf("ApplyReview(edit) = %+v, %v", result, err)
	}
	afterEdit, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	if afterEdit.Cases[0].Decision != DecisionPending || afterEdit.Cases[0].ReviewerID != "" ||
		afterEdit.Cases[0].ContentSHA256 == first.ContentSHA256 {
		t.Fatalf("edit did not invalidate review: %+v", afterEdit.Cases[0])
	}
	if _, err := ApplyReview(root, ReviewInput{
		Action: ReviewActionAccept, CaseID: edited.Case.ID,
		ExpectedSequence: 1, ExpectedContentSHA256: first.ContentSHA256,
		ReviewerID: testReviewerID,
	}); err == nil || !strings.Contains(err.Error(), "reload") {
		t.Fatalf("stale action error = %v", err)
	}
	if _, err := ApplyReview(root, ReviewInput{
		Action: ReviewActionAccept, CaseID: edited.Case.ID,
		ExpectedSequence:      afterEdit.LastSequence,
		ExpectedContentSHA256: afterEdit.Cases[0].ContentSHA256,
		ReviewerID:            testReviewerID,
		Clock:                 func() time.Time { return acceptedAt.Add(2 * time.Minute) },
	}); err != nil {
		t.Fatal(err)
	}
	final, err := LoadReviewState(root)
	if err != nil || final.Cases[0].Decision != DecisionAccepted || final.LastSequence != 3 {
		t.Fatalf("final review state = %+v, %v", final.Cases[0], err)
	}

	stale := ReviewCheckpoint{
		SchemaVersion: CheckpointVersion, CandidateManifestID: final.CandidateManifest.ID,
		LastEventSHA256: strings.Repeat("0", 64), Pending: 650,
	}
	body, _ := marshalCanonical(stale)
	if err := os.WriteFile(filepath.Join(root, ReviewDirectory, ReviewCheckpointFile), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if replayed, err := LoadReviewState(root); err != nil || replayed.LastSequence != 3 {
		t.Fatalf("stale checkpoint displaced ledger authority: sequence=%d err=%v", replayed.LastSequence, err)
	}
}

func TestReviewLedgerRejectsConcurrentStaleAndTamperedEvents(t *testing.T) {
	root := newTestPoolRoot(t)
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	input := ReviewInput{
		Action: ReviewActionAccept, CaseID: state.Cases[0].Snapshot.Case.ID,
		ExpectedSequence: 0, ExpectedContentSHA256: state.Cases[0].ContentSHA256,
		ReviewerID: testReviewerID, Clock: func() time.Time {
			return time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC)
		},
	}
	var wait sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := ApplyReview(root, input)
			errorsSeen <- err
		}()
	}
	wait.Wait()
	close(errorsSeen)
	successes := 0
	for err := range errorsSeen {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successful writes = %d", successes)
	}
	resumed, err := LoadReviewState(root)
	if err != nil || resumed.LastSequence != 1 {
		t.Fatalf("review sequence after concurrency = %d, %v", resumed.LastSequence, err)
	}

	events, err := os.ReadDir(filepath.Join(root, ReviewDirectory, ReviewEventsDirectory))
	if err != nil || len(events) != 1 {
		t.Fatalf("event entries = %d, %v", len(events), err)
	}
	path := filepath.Join(root, ReviewDirectory, ReviewEventsDirectory, events[0].Name())
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)-2] ^= 1
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReviewState(root); err == nil || !strings.Contains(err.Error(), "filename hash") {
		t.Fatalf("tampered event error = %v", err)
	}
}

func TestReviewLedgerRejectsPartialOrForkedDirectoryEntries(t *testing.T) {
	root := newTestPoolRoot(t)
	partial := filepath.Join(root, ReviewDirectory, ReviewEventsDirectory, ".memory-author-partial.tmp")
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReviewState(root); err == nil || !strings.Contains(err.Error(), "filename") {
		t.Fatalf("partial event error = %v", err)
	}
}

func TestReviewEditRejectsGlobalDuplicateBeforePublication(t *testing.T) {
	root := newTestPoolRoot(t)
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	edited := state.Cases[0].Snapshot
	edited.Case.Query = state.Cases[1].Snapshot.Case.Query
	_, err = ApplyReview(root, ReviewInput{
		Action: ReviewActionEdit, CaseID: edited.Case.ID,
		ExpectedSequence:      state.LastSequence,
		ExpectedContentSHA256: state.Cases[0].ContentSHA256,
		ReviewerID:            testReviewerID,
		Snapshot:              &edited,
		Clock: func() time.Time {
			return time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate queries") {
		t.Fatalf("global duplicate edit error = %v", err)
	}
	events, readErr := os.ReadDir(filepath.Join(root, ReviewDirectory, ReviewEventsDirectory))
	if readErr != nil || len(events) != 0 {
		t.Fatalf("rejected edit published %d events: %v", len(events), readErr)
	}
}

func TestReviewReplayRejectsGloballyInvalidEdit(t *testing.T) {
	root := newTestPoolRoot(t)
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	edited := state.Cases[0].Snapshot
	edited.Case.Query = state.Cases[1].Snapshot.Case.Query
	afterHash, err := CaseContentSHA256(edited)
	if err != nil {
		t.Fatal(err)
	}
	fixtureHash, err := fixtureSHA256(edited.Fixture)
	if err != nil {
		t.Fatal(err)
	}
	event := ReviewEvent{
		SchemaVersion:        ReviewEventVersion,
		Sequence:             1,
		PreviousEventSHA256:  state.LastEventSHA256,
		Action:               ReviewActionEdit,
		CaseID:               edited.Case.ID,
		BeforeContentSHA256:  state.Cases[0].ContentSHA256,
		AfterContentSHA256:   afterHash,
		FixtureContentSHA256: fixtureHash,
		ReviewerID:           testReviewerID,
		OccurredAt:           time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Snapshot:             &edited,
	}
	body, err := marshalCanonical(event)
	if err != nil {
		t.Fatal(err)
	}
	eventHash := sha256Hex(body)
	if err := os.WriteFile(
		filepath.Join(root, ReviewDirectory, ReviewEventsDirectory, eventFileName(1, eventHash)),
		body,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReviewState(root); err == nil || !strings.Contains(err.Error(), "duplicate queries") {
		t.Fatalf("globally invalid replay error = %v", err)
	}
}

func TestCurrentStatusUsesEditedCaseCounts(t *testing.T) {
	root := newTestPoolRoot(t)
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	before, err := CurrentStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	caseIndex := -1
	for index, item := range state.Cases {
		if item.Snapshot.Case.Language == "zh" &&
			!contains(item.Snapshot.Case.Slices, "chinese_paraphrase") {
			caseIndex = index
			break
		}
	}
	if caseIndex < 0 {
		t.Fatal("generated pool has no editable status-count witness")
	}
	edited := state.Cases[caseIndex].Snapshot
	originalSplit := edited.Case.Split
	targetSplit := "validation"
	if originalSplit == targetSplit {
		targetSplit = "development"
	}
	edited.Case.Split = targetSplit
	edited.Case.Language = "mixed"
	edited.Case.Slices = append(edited.Case.Slices, "chinese_paraphrase")
	if _, err := ApplyReview(root, ReviewInput{
		Action: ReviewActionEdit, CaseID: edited.Case.ID,
		ExpectedSequence:      state.LastSequence,
		ExpectedContentSHA256: state.Cases[caseIndex].ContentSHA256,
		ReviewerID:            testReviewerID,
		Snapshot:              &edited,
		Clock: func() time.Time {
			return time.Date(2026, time.July, 29, 1, 0, 0, 0, time.UTC)
		},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := CurrentStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if after.LanguageCounts.Chinese != before.LanguageCounts.Chinese-1 ||
		after.LanguageCounts.Mixed != before.LanguageCounts.Mixed+1 {
		t.Fatalf("edited language counts = %+v, before %+v", after.LanguageCounts, before.LanguageCounts)
	}
	if splitCount(after.SplitCounts, originalSplit) != splitCount(before.SplitCounts, originalSplit)-1 ||
		splitCount(after.SplitCounts, targetSplit) != splitCount(before.SplitCounts, targetSplit)+1 {
		t.Fatalf("edited split counts = %+v, before %+v", after.SplitCounts, before.SplitCounts)
	}
	if sliceTotal(after.SliceCounts, "chinese_paraphrase") !=
		sliceTotal(before.SliceCounts, "chinese_paraphrase")+1 {
		t.Fatalf("edited slice counts = %+v, before %+v", after.SliceCounts, before.SliceCounts)
	}
}

func splitCount(counts CountBySplit, split string) int {
	switch split {
	case "development":
		return counts.Development
	case "validation":
		return counts.Validation
	case "holdout":
		return counts.Holdout
	default:
		return -1
	}
}

func sliceTotal(counts []SliceCount, name string) int {
	for _, count := range counts {
		if count.Name == name {
			return count.Total
		}
	}
	return -1
}

func TestReviewWriterLockRejectsSymlink(t *testing.T) {
	root := newTestPoolRoot(t)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("unchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, ReviewDirectory, writerLockFile)); err != nil {
		t.Fatal(err)
	}
	state, err := LoadReviewState(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyReview(root, ReviewInput{
		Action: ReviewActionAccept, CaseID: state.Cases[0].Snapshot.Case.ID,
		ExpectedSequence:      state.LastSequence,
		ExpectedContentSHA256: state.Cases[0].ContentSHA256,
		ReviewerID:            testReviewerID,
	})
	if err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("writer-lock symlink error = %v", err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("writer-lock target mode/error = %v, %v", info.Mode().Perm(), err)
	}
}
