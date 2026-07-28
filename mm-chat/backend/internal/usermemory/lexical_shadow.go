package usermemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var lexicalShadowResultCodeRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

// SearchRelevantWithShadow runs the existing v1 reader first, then compares
// its ordered result with the provider-free PostgreSQL lexical projection.
// Shadow failures are deliberately represented as bounded diagnostics rather
// than main-path errors so the returned v1 items remain authoritative.
func (s *Service) SearchRelevantWithShadow(
	ctx context.Context,
	query string,
	conversationID string,
	assistantMessageID string,
	limit int,
) ([]Memory, LexicalShadowSummary, error) {
	items, err := s.SearchRelevant(ctx, query, limit)
	if err != nil {
		return nil, LexicalShadowSummary{}, err
	}

	failure := lexicalShadowFailure("COMPARE_UNAVAILABLE")
	repository, ok := s.repo.(LexicalShadowRepository)
	if !ok || repository == nil {
		return items, failure, nil
	}
	conversationID = strings.TrimSpace(conversationID)
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if !uuidRE.MatchString(conversationID) || !uuidRE.MatchString(assistantMessageID) ||
		query == "" {
		return items, lexicalShadowFailure("ARGUMENT_INVALID"), nil
	}
	observationID, err := newUUID()
	if err != nil {
		return items, lexicalShadowFailure("OBSERVATION_ID_FAILED"), nil
	}

	baseline := make([]LexicalShadowBaseline, 0, len(items))
	for _, item := range items {
		scopeType := strings.TrimSpace(item.ScopeType)
		if scopeType == "" {
			scopeType = "global"
		}
		baseline = append(baseline, LexicalShadowBaseline{
			MemoryID:  item.ID,
			Revision:  item.Revision,
			ScopeType: scopeType,
		})
	}
	digest := sha256.Sum256([]byte(query))
	summary, err := repository.CompareLexicalShadow(ctx, LexicalShadowInput{
		ObservationID:      observationID,
		ConversationID:     conversationID,
		AssistantMessageID: assistantMessageID,
		QueryHash:          hex.EncodeToString(digest[:]),
		QueryText:          query,
		Baseline:           baseline,
		LexicalLimit:       MaxLexicalShadowResults,
	})
	if err != nil {
		return items, lexicalShadowFailure("COMPARE_FAILED"), nil
	}
	return items, sanitizeLexicalShadowSummary(summary), nil
}

func lexicalShadowFailure(code string) LexicalShadowSummary {
	return LexicalShadowSummary{
		ProfileID:  LexicalShadowProfileID,
		Status:     "failed",
		ResultCode: code,
	}
}

func sanitizeLexicalShadowSummary(summary LexicalShadowSummary) LexicalShadowSummary {
	if summary.ProfileID != LexicalShadowProfileID {
		summary.ProfileID = LexicalShadowProfileID
	}
	switch summary.Status {
	case "pending", "completed", "failed":
	default:
		summary.Status = "failed"
	}
	if !lexicalShadowResultCodeRE.MatchString(summary.ResultCode) {
		summary.ResultCode = "COMPARE_FAILED"
		summary.Status = "failed"
	}
	summary.BaselineCount = clampLexicalShadowCount(summary.BaselineCount, MaxSearchResults)
	summary.ExactCount = clampLexicalShadowCount(summary.ExactCount, 20)
	summary.BM25Count = clampLexicalShadowCount(summary.BM25Count, 30)
	summary.LexicalCount = clampLexicalShadowCount(summary.LexicalCount, MaxLexicalShadowResults)
	summary.OverlapCount = clampLexicalShadowCount(summary.OverlapCount, MaxSearchResults)
	summary.DurationMillis = clampLexicalShadowCount(summary.DurationMillis, 120000)
	return summary
}

func clampLexicalShadowCount(value int, maximum int) int {
	if value < 0 {
		return 0
	}
	return min(value, maximum)
}
