package memoryworker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func buildCaptureProposals(
	job Job,
	capture Capture,
	candidates []rawCaptureCandidate,
	decisions map[int]rawDecision,
) ([]CaptureProposal, error) {
	sourceObservedAt, ok := captureSourceObservedAt(job, capture)
	if !ok {
		return nil, errors.New("memory capture source context is missing")
	}
	messageRoles := make(map[string]string, len(capture.Messages))
	for _, message := range capture.Messages {
		messageRoles[message.ID] = message.Role
	}
	visibleTargets := make(map[string]CaptureMemory, len(capture.CurrentMemories))
	for _, memory := range capture.CurrentMemories {
		visibleTargets[memory.ID] = memory
	}

	result := make([]CaptureProposal, 0, len(candidates))
	for index, raw := range candidates {
		if raw.Importance == nil || raw.Confidence == nil || raw.ScopeConfidence == nil ||
			raw.Tags == nil || raw.AuthorityUserMessageIDs == nil ||
			raw.ContextMessageIDs == nil {
			return nil, errors.New("memory candidate required field is missing")
		}
		if *raw.Importance < 1 || *raw.Importance > 5 {
			return nil, errors.New("memory candidate importance is invalid")
		}
		if err := validateRawTags(raw.Tags); err != nil {
			return nil, err
		}
		basic, normalized, err := usermemory.NormalizeCandidateForStorage(usermemory.Candidate{
			Type: raw.Type, Content: raw.Content, Importance: *raw.Importance, Tags: raw.Tags,
		})
		if err != nil {
			return nil, err
		}
		if *raw.Confidence < 0 || *raw.Confidence > 1 ||
			*raw.ScopeConfidence < 0 || *raw.ScopeConfidence > 1 {
			return nil, errors.New("memory candidate confidence is invalid")
		}
		raw.Sensitivity = strings.ToLower(strings.TrimSpace(raw.Sensitivity))
		modelSensitivity, ok := map[string]int{
			"normal":    sensitivityNormal,
			"sensitive": sensitivitySensitive,
			"secret":    sensitivitySecret,
		}[raw.Sensitivity]
		if !ok {
			return nil, errors.New("memory candidate sensitivity is invalid")
		}
		localSensitivity := classifySensitivity(
			basic.Content + " " + strings.Join(basic.Tags, " "),
		)
		if localSensitivity > modelSensitivity {
			modelSensitivity = localSensitivity
		}
		sensitivity := []string{"normal", "sensitive", "secret"}[modelSensitivity]

		authorityIDs, err := validateEvidenceIDs(
			raw.AuthorityUserMessageIDs, "user", messageRoles, 8,
		)
		if err != nil || !contains(authorityIDs, job.SourceMessageID) {
			return nil, errors.New("memory candidate user authority is invalid")
		}
		contextIDs, err := validateEvidenceIDs(
			raw.ContextMessageIDs, "assistant", messageRoles, 8,
		)
		if err != nil {
			return nil, errors.New("memory candidate assistant context is invalid")
		}
		confirmation := strings.ToLower(strings.TrimSpace(raw.ConfirmationKind))
		if confirmation != "explicit_user" && confirmation != "confirmed_assistant" {
			return nil, errors.New("memory candidate confirmation kind is invalid")
		}
		if confirmation == "confirmed_assistant" && len(contextIDs) == 0 {
			return nil, errors.New("confirmed assistant Memory requires context evidence")
		}

		scopeType := strings.ToLower(strings.TrimSpace(raw.ProposedScopeType))
		scopeConfidence := *raw.ScopeConfidence
		var projectID, conversationID *string
		switch scopeType {
		case "global":
		case "project":
			if capture.ProjectID == "" {
				scopeType = "conversation"
				scopeConfidence = 0
				value := job.SourceConversationID
				conversationID = &value
			} else {
				value := capture.ProjectID
				projectID = &value
			}
		case "conversation":
			value := job.SourceConversationID
			conversationID = &value
		default:
			return nil, errors.New("memory candidate scope is invalid")
		}

		temporalBasis := strings.ToLower(strings.TrimSpace(raw.TemporalBasis))
		if temporalBasis == "" {
			temporalBasis = "none"
		}
		var parserVersion *string
		var validFrom, validTo, factExpiresAt *time.Time
		switch temporalBasis {
		case "none":
			if raw.ValidFrom != nil || raw.ValidTo != nil || raw.FactExpiresAt != nil {
				return nil, errors.New("timeless memory candidate carries temporal values")
			}
		case "source_timestamp":
			version := "source-message-v1"
			parserVersion = &version
			if raw.ValidFrom != nil || raw.ValidTo != nil || raw.FactExpiresAt != nil {
				return nil, errors.New("source timestamp candidate carries model time values")
			}
		case "explicit_absolute":
			validFrom, err = parseOptionalRFC3339(raw.ValidFrom)
			if err != nil {
				return nil, err
			}
			validTo, err = parseOptionalRFC3339(raw.ValidTo)
			if err != nil {
				return nil, err
			}
			factExpiresAt, err = parseOptionalRFC3339(raw.FactExpiresAt)
			if err != nil {
				return nil, err
			}
			if validFrom == nil && validTo == nil && factExpiresAt == nil {
				return nil, errors.New("absolute temporal candidate has no absolute value")
			}
			version := "rfc3339-v1"
			parserVersion = &version
		case "relative_ambiguous", "model_inferred":
			version := "unresolved-v1"
			parserVersion = &version
			validFrom, validTo, factExpiresAt = nil, nil, nil
		default:
			return nil, errors.New("memory candidate temporal basis is invalid")
		}
		if validFrom != nil && validTo != nil && validTo.Before(*validFrom) {
			return nil, errors.New("memory candidate validity interval is invalid")
		}
		if validFrom != nil && factExpiresAt != nil && factExpiresAt.Before(*validFrom) {
			return nil, errors.New("memory candidate expiry is invalid")
		}

		decision, ok := decisions[index+1]
		if !ok || decision.Ordinal == nil {
			return nil, errors.New("memory candidate decision is missing")
		}
		targetIDs := make([]string, 0, len(decision.TargetMemoryIDs))
		for _, targetID := range decision.TargetMemoryIDs {
			memory, ok := visibleTargets[targetID]
			if !ok {
				return nil, errors.New("memory candidate decision target is invalid")
			}
			if sameProposalScope(memory, scopeType, projectID, conversationID) {
				targetIDs = append(targetIDs, targetID)
			}
		}
		action := strings.ToUpper(strings.TrimSpace(decision.Action))
		if len(decision.TargetMemoryIDs) > 0 && len(targetIDs) == 0 {
			action = "ADD"
		}

		id, err := chat.NewUUID()
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256([]byte(basic.Content))
		content, normalizedContent := &basic.Content, &normalized
		tags := append([]string(nil), basic.Tags...)
		subjectKey, err := normalizeOptionalKey(raw.SubjectKey)
		if err != nil {
			return nil, err
		}
		factKey, err := normalizeOptionalKey(raw.FactKey)
		if err != nil {
			return nil, err
		}
		if modelSensitivity == sensitivitySecret {
			content, normalizedContent = nil, nil
			tags = []string{}
			subjectKey, factKey = nil, nil
			action = "REJECT"
			targetIDs = []string{}
		}
		result = append(result, CaptureProposal{
			ID: id, Type: basic.Type, Content: content,
			NormalizedContent: normalizedContent,
			CandidateHash:     hex.EncodeToString(digest[:]),
			Importance:        basic.Importance, Tags: tags,
			SubjectKey: subjectKey, FactKey: factKey,
			Sensitivity: sensitivity, Confidence: *raw.Confidence,
			ConfidenceBand:          confidenceBand(*raw.Confidence),
			AuthorityUserMessageIDs: authorityIDs,
			ContextMessageIDs:       contextIDs,
			ConfirmationKind:        confirmation,
			ProposedScopeType:       scopeType, ProposedProjectID: projectID,
			ProposedConversationID: conversationID,
			ScopeConfidence:        scopeConfidence,
			TemporalBasis:          temporalBasis, TemporalParserVersion: parserVersion,
			ObservedAt: sourceObservedAt.UTC(), ValidFrom: validFrom,
			ValidTo: validTo, FactExpiresAt: factExpiresAt,
			ProposedAction: action, TargetMemoryIDs: targetIDs,
		})
	}
	return result, nil
}

func captureSourceObservedAt(job Job, capture Capture) (time.Time, bool) {
	for _, message := range capture.Messages {
		if message.ID == job.SourceMessageID && message.Role == "user" {
			return message.ObservedAt, !message.ObservedAt.IsZero()
		}
	}
	return time.Time{}, false
}

func validateEvidenceIDs(
	values []string,
	role string,
	messageRoles map[string]string,
	limit int,
) ([]string, error) {
	if len(values) > limit {
		return nil, errors.New("memory candidate evidence count is invalid")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !uuidRE.MatchString(value) || messageRoles[value] != role {
			return nil, errors.New("memory candidate evidence id is invalid")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, errors.New("memory candidate evidence id is duplicated")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func sameProposalScope(
	memory CaptureMemory,
	scopeType string,
	projectID *string,
	conversationID *string,
) bool {
	if memory.ScopeType != scopeType {
		return false
	}
	switch scopeType {
	case "global":
		return memory.ProjectID == "" && memory.ConversationID == ""
	case "project":
		return projectID != nil && memory.ProjectID == *projectID
	case "conversation":
		return conversationID != nil && memory.ConversationID == *conversationID
	default:
		return false
	}
}

func parseOptionalRFC3339(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		return nil, errors.New("memory candidate absolute time is invalid")
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func normalizeOptionalKey(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(*value), " "))
	if normalized == "" {
		return nil, nil
	}
	if len(normalized) > 256 {
		return nil, errors.New("memory candidate key is invalid")
	}
	return &normalized, nil
}

func validateRawTags(tags []string) error {
	if len(tags) > usermemory.MaxTags {
		return errors.New("memory candidate tag count is invalid")
	}
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		normalized := strings.ToLower(strings.Join(strings.Fields(tag), " "))
		if normalized == "" || utf8.RuneCountInString(normalized) > usermemory.MaxTagChars {
			return errors.New("memory candidate tag is invalid")
		}
		if _, duplicate := seen[normalized]; duplicate {
			return errors.New("memory candidate tag is duplicated")
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

func confidenceBand(value float64) string {
	if value < 0.5 {
		return "low"
	}
	if value < 0.8 {
		return "medium"
	}
	return "high"
}

func proposalValidationFailureCategory(err error) string {
	if err == nil {
		return "PROPOSAL_VALIDATION_INVALID"
	}
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "source context"):
		return "PROPOSAL_SOURCE_CONTEXT_INVALID"
	case strings.Contains(value, "required field"):
		return "PROPOSAL_REQUIRED_FIELD_INVALID"
	case strings.Contains(value, "importance"):
		return "PROPOSAL_IMPORTANCE_INVALID"
	case strings.Contains(value, "tag"):
		return "PROPOSAL_TAG_INVALID"
	case strings.Contains(value, "content") || strings.Contains(value, "memory type"):
		return "PROPOSAL_CONTENT_INVALID"
	case strings.Contains(value, "confidence"):
		return "PROPOSAL_CONFIDENCE_INVALID"
	case strings.Contains(value, "sensitivity"):
		return "PROPOSAL_SENSITIVITY_INVALID"
	case strings.Contains(value, "user authority"):
		return "PROPOSAL_USER_AUTHORITY_INVALID"
	case strings.Contains(value, "assistant context"):
		return "PROPOSAL_ASSISTANT_CONTEXT_INVALID"
	case strings.Contains(value, "confirmation") ||
		strings.Contains(value, "confirmed assistant"):
		return "PROPOSAL_CONFIRMATION_INVALID"
	case strings.Contains(value, "scope"):
		return "PROPOSAL_SCOPE_INVALID"
	case strings.Contains(value, "temporal") || strings.Contains(value, "timestamp") ||
		strings.Contains(value, "absolute time") || strings.Contains(value, "validity") ||
		strings.Contains(value, "expiry") || strings.Contains(value, "timeless"):
		return "PROPOSAL_TEMPORAL_INVALID"
	case strings.Contains(value, "decision"):
		return "PROPOSAL_DECISION_INVALID"
	case strings.Contains(value, "key"):
		return "PROPOSAL_KEY_INVALID"
	case strings.Contains(value, "evidence"):
		return "PROPOSAL_EVIDENCE_INVALID"
	default:
		return "PROPOSAL_VALIDATION_INVALID"
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func proposalProfile(base string, version string) string {
	digest := sha256.Sum256([]byte(base + "\x1f" + version))
	return hex.EncodeToString(digest[:])
}
