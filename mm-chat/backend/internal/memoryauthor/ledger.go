package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const writerLockFile = "writer.lock"

var reviewEventName = regexp.MustCompile(`^([0-9]{8})-([0-9a-f]{64})\.json$`)

type ReviewInput struct {
	Action                ReviewAction
	CaseID                string
	ExpectedSequence      uint64
	ExpectedContentSHA256 string
	ReviewerID            string
	Snapshot              *CaseSnapshot
	Clock                 func() time.Time
}

type ReviewResult struct {
	Sequence      uint64   `json:"sequence"`
	EventSHA256   string   `json:"eventSha256"`
	CaseID        string   `json:"caseId"`
	ContentSHA256 string   `json:"contentSha256"`
	Decision      Decision `json:"decision"`
}

func LoadReviewState(root string) (ReviewState, error) {
	pool, err := LoadPool(root)
	if err != nil {
		return ReviewState{}, err
	}
	fixtures := make(map[string]Fixture, len(pool.FixtureManifest.Fixtures))
	for _, fixture := range pool.FixtureManifest.Fixtures {
		fixtures[fixture.Alias] = fixture
	}
	state := ReviewState{
		CandidateManifest: pool.Manifest,
		FixtureManifest:   pool.FixtureManifest,
		Golden:            pool.Golden,
		LastEventSHA256:   strings.Repeat("0", 64),
		Cases:             make([]CaseState, 0, len(pool.Golden.Cases)),
	}
	caseIndex := make(map[string]int, len(pool.Golden.Cases))
	for _, item := range pool.Golden.Cases {
		fixture, ok := fixtures[item.FixtureAlias]
		if !ok {
			return ReviewState{}, fmt.Errorf("candidate fixture %q is missing", item.FixtureAlias)
		}
		snapshot := CaseSnapshot{Case: item, Fixture: fixture}
		digest, err := CaseContentSHA256(snapshot)
		if err != nil {
			return ReviewState{}, err
		}
		caseIndex[item.ID] = len(state.Cases)
		state.Cases = append(state.Cases, CaseState{
			Snapshot: snapshot, ContentSHA256: digest, Decision: DecisionPending,
		})
	}
	eventsPath := filepath.Join(root, ReviewDirectory, ReviewEventsDirectory)
	if err := validateSecureDirectory(eventsPath); err != nil {
		return ReviewState{}, fmt.Errorf("validate review events directory: %w", err)
	}
	entries, err := os.ReadDir(eventsPath)
	if err != nil {
		return ReviewState{}, errors.New("read review events directory")
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	expectedSequence := uint64(1)
	previousHash := strings.Repeat("0", 64)
	var previousOccurredAt time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			return ReviewState{}, errors.New("review event directory contains an unexpected entry")
		}
		matches := reviewEventName.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			return ReviewState{}, fmt.Errorf("review event filename %q is invalid", entry.Name())
		}
		nameSequence, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil || nameSequence != expectedSequence {
			return ReviewState{}, errors.New("review event sequence is gapped or forked")
		}
		body, err := readSecureArtifact(filepath.Join(eventsPath, entry.Name()))
		if err != nil {
			return ReviewState{}, fmt.Errorf("read review event %d: %w", expectedSequence, err)
		}
		eventHash := sha256Hex(body)
		if eventHash != matches[2] {
			return ReviewState{}, fmt.Errorf("review event %d filename hash does not match", expectedSequence)
		}
		event, err := decodeReviewEvent(body)
		if err != nil {
			return ReviewState{}, fmt.Errorf("review event %d: %w", expectedSequence, err)
		}
		if event.Sequence != expectedSequence || event.PreviousEventSHA256 != previousHash {
			return ReviewState{}, errors.New("review event hash chain is gapped or forked")
		}
		eventOccurredAt, _ := time.Parse(time.RFC3339, event.OccurredAt)
		if !previousOccurredAt.IsZero() && eventOccurredAt.Before(previousOccurredAt) {
			return ReviewState{}, errors.New("review event time moved backwards")
		}
		index, ok := caseIndex[event.CaseID]
		if !ok {
			return ReviewState{}, fmt.Errorf("review event references unknown case %q", event.CaseID)
		}
		if event.Action == ReviewActionEdit {
			current := state.Cases[index].Snapshot
			if event.Snapshot == nil || event.Snapshot.Case.ID != current.Case.ID ||
				event.Snapshot.Case.FixtureAlias != current.Case.FixtureAlias ||
				event.Snapshot.Fixture.Alias != current.Fixture.Alias {
				return ReviewState{}, errors.New("review edit changes an immutable case identifier")
			}
			if err := validateSnapshot(*event.Snapshot, state.Golden); err != nil {
				return ReviewState{}, fmt.Errorf("review edit snapshot: %w", err)
			}
		}
		if err := applyEvent(&state.Cases[index], event); err != nil {
			return ReviewState{}, fmt.Errorf("review event %d: %w", expectedSequence, err)
		}
		previousHash = eventHash
		previousOccurredAt = eventOccurredAt
		expectedSequence++
	}
	state.LastSequence = expectedSequence - 1
	state.LastEventSHA256 = previousHash
	if !previousOccurredAt.IsZero() {
		state.LastOccurredAt = previousOccurredAt.Format(time.RFC3339)
	}
	if _, err := validateReviewStateContent(state); err != nil {
		return ReviewState{}, fmt.Errorf("validate current review state: %w", err)
	}
	if err := validateExistingCheckpoint(root, state); err != nil {
		return ReviewState{}, err
	}
	return state, nil
}

func ApplyReview(root string, input ReviewInput) (ReviewResult, error) {
	var result ReviewResult
	err := withWriterLock(root, func() error {
		if frozenExists(root) {
			return errors.New("frozen corpus cannot be reviewed or edited")
		}
		state, err := LoadReviewState(root)
		if err != nil {
			return err
		}
		if input.ExpectedSequence != state.LastSequence {
			return errors.New("review state changed; reload before submitting")
		}
		caseIndex := -1
		for index := range state.Cases {
			if state.Cases[index].Snapshot.Case.ID == input.CaseID {
				caseIndex = index
				break
			}
		}
		if caseIndex < 0 {
			return errors.New("review case is unknown")
		}
		current := state.Cases[caseIndex]
		if input.ExpectedContentSHA256 != current.ContentSHA256 {
			return errors.New("case content changed; reload before submitting")
		}
		clock := input.Clock
		if clock == nil {
			clock = time.Now
		}
		occurredAt := clock().UTC().Truncate(time.Second)
		if state.LastOccurredAt != "" {
			lastOccurredAt, _ := time.Parse(time.RFC3339, state.LastOccurredAt)
			if occurredAt.Before(lastOccurredAt) {
				return errors.New("review clock moved backwards; refusing non-monotonic evidence")
			}
		}
		event := ReviewEvent{
			SchemaVersion:       ReviewEventVersion,
			Sequence:            state.LastSequence + 1,
			PreviousEventSHA256: state.LastEventSHA256,
			Action:              input.Action,
			CaseID:              input.CaseID,
			BeforeContentSHA256: current.ContentSHA256,
			AfterContentSHA256:  current.ContentSHA256,
			ReviewerID:          strings.TrimSpace(input.ReviewerID),
			OccurredAt:          occurredAt.Format(time.RFC3339),
		}
		fixtureHash, err := fixtureSHA256(current.Snapshot.Fixture)
		if err != nil {
			return err
		}
		event.FixtureContentSHA256 = fixtureHash
		switch input.Action {
		case ReviewActionAccept, ReviewActionReject:
			if input.Snapshot != nil {
				return errors.New("accept/reject cannot include edited content")
			}
		case ReviewActionEdit:
			if input.Snapshot == nil {
				return errors.New("edit requires a complete case snapshot")
			}
			edited := *input.Snapshot
			if edited.Case.ID != current.Snapshot.Case.ID ||
				edited.Case.FixtureAlias != current.Snapshot.Case.FixtureAlias ||
				edited.Fixture.Alias != current.Snapshot.Fixture.Alias {
				return errors.New("case ID and fixture alias are immutable")
			}
			if err := validateSnapshot(edited, state.Golden); err != nil {
				return err
			}
			event.Snapshot = &edited
			event.AfterContentSHA256, err = CaseContentSHA256(edited)
			if err != nil {
				return err
			}
			if event.AfterContentSHA256 == event.BeforeContentSHA256 {
				return errors.New("edit does not change case content")
			}
			event.FixtureContentSHA256, err = fixtureSHA256(edited.Fixture)
			if err != nil {
				return err
			}
		default:
			return errors.New("review action is invalid")
		}
		if err := validateReviewEvent(event); err != nil {
			return err
		}
		if event.Action == ReviewActionEdit {
			tentative := state
			tentative.Cases = append([]CaseState(nil), state.Cases...)
			if err := applyEvent(&tentative.Cases[caseIndex], event); err != nil {
				return err
			}
			if _, err := validateReviewStateContent(tentative); err != nil {
				return fmt.Errorf("review edit would invalidate current candidate set: %w", err)
			}
		}
		body, err := marshalCanonical(event)
		if err != nil {
			return err
		}
		eventHash := sha256Hex(body)
		name := fmt.Sprintf("%08d-%s.json", event.Sequence, eventHash)
		if err := writeExclusiveBytesStaged(
			filepath.Join(root, ReviewDirectory, ReviewEventsDirectory, name), body,
			filepath.Join(root, ReviewDirectory),
		); err != nil {
			return fmt.Errorf("append review event: %w", err)
		}
		if err := applyEvent(&state.Cases[caseIndex], event); err != nil {
			return err
		}
		state.LastSequence = event.Sequence
		state.LastEventSHA256 = eventHash
		state.LastOccurredAt = event.OccurredAt
		if err := writeCheckpoint(root, state); err != nil {
			return fmt.Errorf("review event committed but checkpoint update failed: %w", err)
		}
		result = ReviewResult{
			Sequence: event.Sequence, EventSHA256: eventHash, CaseID: event.CaseID,
			ContentSHA256: state.Cases[caseIndex].ContentSHA256,
			Decision:      state.Cases[caseIndex].Decision,
		}
		return nil
	})
	return result, err
}

func applyEvent(state *CaseState, event ReviewEvent) error {
	if state == nil || event.BeforeContentSHA256 != state.ContentSHA256 {
		return errors.New("review event is bound to stale case content")
	}
	currentFixtureHash, err := fixtureSHA256(state.Snapshot.Fixture)
	if err != nil {
		return err
	}
	switch event.Action {
	case ReviewActionEdit:
		if event.Snapshot == nil || event.AfterContentSHA256 == event.BeforeContentSHA256 {
			return errors.New("review edit payload is invalid")
		}
		digest, err := CaseContentSHA256(*event.Snapshot)
		if err != nil || digest != event.AfterContentSHA256 {
			return errors.New("review edit content hash does not match")
		}
		fixtureHash, err := fixtureSHA256(event.Snapshot.Fixture)
		if err != nil || fixtureHash != event.FixtureContentSHA256 {
			return errors.New("review edit fixture hash does not match")
		}
		state.Snapshot = *event.Snapshot
		state.ContentSHA256 = digest
		state.Decision = DecisionPending
		state.ReviewerID = ""
		state.ReviewedAt = ""
	case ReviewActionAccept, ReviewActionReject:
		if event.Snapshot != nil || event.AfterContentSHA256 != event.BeforeContentSHA256 ||
			event.FixtureContentSHA256 != currentFixtureHash {
			return errors.New("review decision content binding is invalid")
		}
		if event.Action == ReviewActionAccept {
			state.Decision = DecisionAccepted
		} else {
			state.Decision = DecisionRejected
		}
		state.ReviewerID = event.ReviewerID
		state.ReviewedAt = event.OccurredAt
	default:
		return errors.New("review event action is invalid")
	}
	return nil
}

func validateReviewEvent(event ReviewEvent) error {
	if event.SchemaVersion != ReviewEventVersion || event.Sequence == 0 ||
		!validSHA256(event.PreviousEventSHA256) || !validIdentifier(event.CaseID) ||
		!validSHA256(event.BeforeContentSHA256) || !validSHA256(event.AfterContentSHA256) ||
		!validSHA256(event.FixtureContentSHA256) || !validUUID(event.ReviewerID) ||
		!validTimestamp(event.OccurredAt) {
		return errors.New("review event header is invalid")
	}
	switch event.Action {
	case ReviewActionAccept, ReviewActionReject:
		if event.Snapshot != nil || event.BeforeContentSHA256 != event.AfterContentSHA256 {
			return errors.New("review decision payload is invalid")
		}
	case ReviewActionEdit:
		if event.Snapshot == nil || event.BeforeContentSHA256 == event.AfterContentSHA256 {
			return errors.New("review edit payload is invalid")
		}
	default:
		return errors.New("review event action is invalid")
	}
	return nil
}

func validateSnapshot(snapshot CaseSnapshot, template memoryeval.GoldenSet) error {
	if snapshot.Case.Review != (memoryeval.Review{State: "draft"}) {
		return errors.New("edited case cannot carry review attestation")
	}
	promotion := false
	fixtures := FixtureManifest{
		SchemaVersion: FixtureSchemaVersion, ID: "snapshot-fixture",
		Description:       "Synthetic review snapshot validation fixture.",
		PromotionEligible: &promotion, DataPolicy: DataPolicy{SyntheticOnly: true},
		Generator: expectedGenerator(), ContentSHA256: strings.Repeat("0", 64),
		Fixtures: []Fixture{snapshot.Fixture},
	}
	digest, err := FixtureContentSHA256(fixtures)
	if err != nil {
		return err
	}
	fixtures.ContentSHA256 = digest
	template.ID = "snapshot-golden"
	template.Description = "Synthetic review snapshot validation Golden."
	template.PromotionEligible = &promotion
	template.FixtureManifestSHA256 = digest
	template.Lifecycle = memoryeval.GoldenLifecycle{State: "draft"}
	template.Cases = []memoryeval.GoldenCase{snapshot.Case}
	body, err := marshalCanonical(template)
	if err != nil {
		return err
	}
	decoded, err := memoryeval.DecodeGoldenSet(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("validate edited Golden case: %w", err)
	}
	if err := validateFixtureManifest(fixtures); err != nil {
		return err
	}
	return validateGoldenFixtureBinding(fixtures, decoded)
}

func validateExistingCheckpoint(root string, state ReviewState) error {
	path := filepath.Join(root, ReviewDirectory, ReviewCheckpointFile)
	body, err := readSecureArtifact(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read review checkpoint: %w", err)
	}
	checkpoint, err := decodeCheckpoint(body)
	if err != nil {
		return err
	}
	if checkpoint.CandidateManifestID != state.CandidateManifest.ID {
		return errors.New("review checkpoint candidate binding does not match")
	}
	// A valid but stale checkpoint is intentionally tolerated. The immutable
	// ledger remains authority and the next successful mutation rebuilds it.
	return nil
}

func writeCheckpoint(root string, state ReviewState) error {
	accepted, rejected, pending := decisionCounts(state.Cases)
	checkpoint := ReviewCheckpoint{
		SchemaVersion:       CheckpointVersion,
		CandidateManifestID: state.CandidateManifest.ID,
		LastSequence:        state.LastSequence,
		LastEventSHA256:     state.LastEventSHA256,
		Accepted:            accepted, Rejected: rejected, Pending: pending,
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return err
	}
	body, err := marshalCanonical(checkpoint)
	if err != nil {
		return err
	}
	return replacePrivateBytes(filepath.Join(root, ReviewDirectory, ReviewCheckpointFile), body)
}

func validateCheckpoint(checkpoint ReviewCheckpoint) error {
	if checkpoint.SchemaVersion != CheckpointVersion ||
		!validIdentifier(checkpoint.CandidateManifestID) ||
		!validSHA256(checkpoint.LastEventSHA256) ||
		checkpoint.Accepted < 0 || checkpoint.Rejected < 0 || checkpoint.Pending < 0 ||
		checkpoint.Accepted+checkpoint.Rejected+checkpoint.Pending != 650 {
		return errors.New("review checkpoint is invalid")
	}
	if checkpoint.LastSequence == 0 && checkpoint.LastEventSHA256 != strings.Repeat("0", 64) {
		return errors.New("empty review checkpoint hash is invalid")
	}
	return nil
}

func withWriterLock(root string, action func() error) error {
	reviewPath := filepath.Join(root, ReviewDirectory)
	if err := validateSecureDirectory(reviewPath); err != nil {
		return err
	}
	path := filepath.Join(reviewPath, writerLockFile)
	var before os.FileInfo
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
			info.Mode().Perm()&0o077 != 0 {
			return errors.New("review writer lock must be a private regular non-symlink file")
		}
		before = info
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("inspect review writer lock")
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return errors.New("open review writer lock")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() ||
		(before != nil && !os.SameFile(before, opened)) {
		return errors.New("review writer lock changed while opening")
	}
	if err := file.Chmod(0o600); err != nil {
		return errors.New("secure review writer lock")
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("another review writer is active")
	}
	defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return action()
}

func decisionCounts(cases []CaseState) (accepted, rejected, pending int) {
	for _, item := range cases {
		switch item.Decision {
		case DecisionAccepted:
			accepted++
		case DecisionRejected:
			rejected++
		default:
			pending++
		}
	}
	return accepted, rejected, pending
}
