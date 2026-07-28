package usermemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	resultCodeRE = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)
	sha256HexRE  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

var directActions = map[string]struct{}{
	"remember": {},
	"correct":  {},
	"forget":   {},
}

var directActionScopes = map[string]struct{}{
	"global":       {},
	"project":      {},
	"conversation": {},
}

func (s *Service) HydrateDirectAction(
	ctx context.Context,
	input DirectActionHydrationInput,
) (DirectActionContext, error) {
	repo, err := s.requireActionRepository()
	if err != nil {
		return DirectActionContext{}, err
	}
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.SourceMessageID = strings.TrimSpace(input.SourceMessageID)
	input.AssistantMessageID = strings.TrimSpace(input.AssistantMessageID)
	if !uuidRE.MatchString(input.ConversationID) ||
		!uuidRE.MatchString(input.SourceMessageID) ||
		!uuidRE.MatchString(input.AssistantMessageID) {
		return DirectActionContext{}, validation(
			"INVALID_MEMORY_ACTION_SOURCE",
			"memory action source ids must be UUIDs",
		)
	}
	result, err := repo.HydrateDirectAction(ctx, input)
	if err != nil {
		return DirectActionContext{}, err
	}
	if result.Memories == nil {
		result.Memories = []DirectActionMemory{}
	}
	return result, nil
}

func (s *Service) ExecuteDirectAction(
	ctx context.Context,
	input DirectActionExecution,
) (DirectActionResult, error) {
	repo, err := s.requireActionRepository()
	if err != nil {
		return DirectActionResult{}, err
	}
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.SourceMessageID = strings.TrimSpace(input.SourceMessageID)
	input.AssistantMessageID = strings.TrimSpace(input.AssistantMessageID)
	if !uuidRE.MatchString(input.ConversationID) ||
		!uuidRE.MatchString(input.SourceMessageID) ||
		!uuidRE.MatchString(input.AssistantMessageID) {
		return DirectActionResult{}, validation(
			"INVALID_MEMORY_ACTION_SOURCE",
			"memory action source ids must be UUIDs",
		)
	}
	input.RequestedAction = strings.ToLower(strings.TrimSpace(input.RequestedAction))
	if _, ok := directActions[input.RequestedAction]; !ok {
		return DirectActionResult{}, validation(
			"INVALID_MEMORY_ACTION", "memory action is invalid",
		)
	}
	input.ScopeType = strings.ToLower(strings.TrimSpace(input.ScopeType))
	if _, ok := directActionScopes[input.ScopeType]; !ok {
		return DirectActionResult{}, validation(
			"INVALID_MEMORY_ACTION_SCOPE", "memory action scope is invalid",
		)
	}
	if input.Confidence < 0 || input.Confidence > 1 {
		return DirectActionResult{}, validation(
			"INVALID_MEMORY_ACTION_CONFIDENCE", "memory action confidence is invalid",
		)
	}
	if len(input.Targets) > MaxActionTargets {
		return DirectActionResult{}, validation(
			"INVALID_MEMORY_ACTION_TARGET", "memory action has too many targets",
		)
	}
	seenTargets := make(map[string]struct{}, len(input.Targets))
	for index := range input.Targets {
		target := &input.Targets[index]
		target.MemoryID = strings.TrimSpace(target.MemoryID)
		if !uuidRE.MatchString(target.MemoryID) || target.ExpectedRevision < 1 {
			return DirectActionResult{}, validation(
				"INVALID_MEMORY_ACTION_TARGET", "memory action target is invalid",
			)
		}
		if _, duplicate := seenTargets[target.MemoryID]; duplicate {
			return DirectActionResult{}, validation(
				"INVALID_MEMORY_ACTION_TARGET", "memory action target is duplicated",
			)
		}
		seenTargets[target.MemoryID] = struct{}{}
	}

	apply := DirectActionApplyInput{
		ConversationID:      input.ConversationID,
		SourceMessageID:     input.SourceMessageID,
		AssistantMessageID:  input.AssistantMessageID,
		SchemaMajor:         DirectActionSchemaMajor,
		RequestedAction:     input.RequestedAction,
		Sensitivity:         strings.ToLower(strings.TrimSpace(input.Sensitivity)),
		ScopeType:           input.ScopeType,
		Confidence:          input.Confidence,
		Targets:             make([]DirectActionTarget, len(input.Targets)),
		PreflightStatus:     strings.ToLower(strings.TrimSpace(input.PreflightStatus)),
		PreflightResultCode: strings.ToUpper(strings.TrimSpace(input.PreflightResultCode)),
	}
	copy(apply.Targets, input.Targets)
	if apply.Sensitivity == "" {
		apply.Sensitivity = SensitivityNormal
	}
	if apply.PreflightStatus != "" {
		switch apply.PreflightStatus {
		case "review_required", "rejected", "failed":
		default:
			return DirectActionResult{}, validation(
				"INVALID_MEMORY_ACTION_OUTCOME", "memory action outcome is invalid",
			)
		}
		if !resultCodeRE.MatchString(apply.PreflightResultCode) {
			return DirectActionResult{}, validation(
				"INVALID_MEMORY_ACTION_OUTCOME", "memory action result code is invalid",
			)
		}
	}

	if input.Candidate != nil && input.RequestedAction != "forget" {
		normalized, normalizeErr := normalizeCandidate(*input.Candidate)
		if normalizeErr != nil {
			return DirectActionResult{}, normalizeErr
		}
		localSensitivity := ClassifyMemorySensitivity(
			normalized.Content + " " + strings.Join(normalized.Tags, " "),
		)
		if localSensitivity == SensitivitySecret || apply.Sensitivity == SensitivitySecret {
			apply.Sensitivity = SensitivitySecret
			apply.PreflightStatus = "rejected"
			apply.PreflightResultCode = "SECRET_REJECTED"
			apply.CandidateHash = hashMemoryActionValue(normalized.Content)
		} else {
			if localSensitivity == SensitivitySensitive {
				apply.Sensitivity = SensitivitySensitive
			}
			apply.MemoryType = normalized.Type
			apply.Content = normalized.Content
			apply.NormalizedContent = normalizeSearchText(normalized.Content)
			apply.CandidateHash = hashMemoryActionValue(normalized.Content)
			apply.Importance = normalized.Importance
			apply.Tags = normalized.Tags
		}
	}
	if apply.CandidateHash == "" {
		apply.CandidateHash = strings.ToLower(strings.TrimSpace(input.RequestHash))
	}
	if !sha256HexRE.MatchString(apply.CandidateHash) {
		return DirectActionResult{}, validation(
			"INVALID_MEMORY_ACTION_HASH", "memory action hash is invalid",
		)
	}

	ids := make([]string, 7)
	for index := range ids {
		ids[index], err = newUUID()
		if err != nil {
			return DirectActionResult{}, err
		}
	}
	apply.ActionID = ids[0]
	apply.ActivityID = ids[1]
	apply.MemoryID = ids[2]
	apply.EventID = ids[3]
	apply.JobID = ids[4]
	apply.TombstoneID = ids[5]
	apply.ManifestID = ids[6]

	result, err := repo.ApplyDirectAction(ctx, apply)
	if err != nil {
		return DirectActionResult{}, err
	}
	result.Action = input.RequestedAction
	return result, nil
}

func (s *Service) ListActivities(
	ctx context.Context,
	cursor string,
	limit int,
) ([]MemoryActivity, error) {
	repo, err := s.requireActionRepository()
	if err != nil {
		return nil, err
	}
	cursor = strings.TrimSpace(cursor)
	if cursor != "" && !uuidRE.MatchString(cursor) {
		return nil, validation("INVALID_MEMORY_ACTIVITY_CURSOR", "activity cursor is invalid")
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > MaxActivityPage {
		return nil, validation("INVALID_MEMORY_ACTIVITY_LIMIT", "activity limit is invalid")
	}
	items, err := repo.ListActivities(ctx, cursor, limit)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []MemoryActivity{}
	}
	return items, nil
}

func (s *Service) ListMessageUsages(
	ctx context.Context,
	assistantMessageID string,
) ([]MessageMemoryUsage, error) {
	repo, err := s.requireActionRepository()
	if err != nil {
		return nil, err
	}
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(assistantMessageID) {
		return nil, validation("INVALID_ASSISTANT_MESSAGE_ID", "assistant message id is invalid")
	}
	items, err := repo.ListMessageUsages(ctx, assistantMessageID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []MessageMemoryUsage{}
	}
	return items, nil
}

func (s *Service) UndoActivity(
	ctx context.Context,
	activityID string,
	expectedRevision int64,
) (UndoActivityResult, error) {
	repo, err := s.requireActionRepository()
	if err != nil {
		return UndoActivityResult{}, err
	}
	activityID = strings.TrimSpace(activityID)
	if !uuidRE.MatchString(activityID) || expectedRevision < 1 {
		return UndoActivityResult{}, validation(
			"INVALID_MEMORY_ACTIVITY_UNDO", "activity undo input is invalid",
		)
	}
	ids := make([]string, 4)
	for index := range ids {
		ids[index], err = newUUID()
		if err != nil {
			return UndoActivityResult{}, err
		}
	}
	return repo.UndoActivity(ctx, UndoActivityInput{
		ActivityID: activityID, ExpectedRevision: expectedRevision,
		EventID: ids[0], JobID: ids[1], TombstoneID: ids[2], ManifestID: ids[3],
	})
}

func (s *Service) requireActionRepository() (ActionRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(ActionRepository)
	if !ok || repo == nil {
		return nil, ErrActionRepositoryRequired
	}
	return repo, nil
}

func hashMemoryActionValue(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
