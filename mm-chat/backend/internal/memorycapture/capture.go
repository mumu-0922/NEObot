package memorycapture

import (
	"context"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

// CaptureBaseline executes the actual v1 Global-only Top-5 reader.
func CaptureBaseline(
	ctx context.Context,
	service *usermemory.Service,
	index FixtureIndex,
	input RuntimeCase,
) (memoryeval.CaseObservation, error) {
	if service == nil || !validRuntimeCase(input) {
		return memoryeval.CaseObservation{}, ErrCaptureInvalid
	}
	ctx = auth.WithUser(ctx, auth.User{ID: input.UserID})
	started := time.Now()
	items, err := service.SearchRelevant(ctx, input.Query, usermemory.MaxSearchResults)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return memoryeval.CaseObservation{}, errors.Join(ErrCaptureUnavailable, err)
	}
	databaseIDs := make([]string, len(items))
	for position, item := range items {
		databaseIDs[position] = item.ID
	}
	opaque, err := index.OpaqueMemoryIDs(databaseIDs)
	if err != nil {
		return memoryeval.CaseObservation{}, err
	}
	fallback := "none"
	if len(opaque) == 0 {
		fallback = "no_memory"
	}
	return memoryeval.CaseObservation{
		CaseID: input.CaseID, CandidateMemoryIDs: opaque,
		FinalMemoryIDs:     append([]string(nil), opaque...),
		InjectedMemoryIDs:  append([]string(nil), opaque...),
		PersistedMemoryIDs: []string{}, ProviderSentMemoryIDs: []string{},
		LatencyMilliseconds: latency,
		PromptMemoryTokens:  usermemory.EstimatePromptMemoryTokens(items),
		HardCutoffApplied:   latency >= 2000,
		Fallback:            fallback,
	}, nil
}

// CaptureCandidate executes the production hybrid shadow and treats its final
// set as a counterfactual offline injection surface. Production prompt/Usage
// authority remains unchanged.
func CaptureCandidate(
	ctx context.Context,
	service *usermemory.Service,
	recorder *Recorder,
	index FixtureIndex,
	input RuntimeCase,
) (memoryeval.CaseObservation, error) {
	observed, _, err := captureCandidateWithCalibration(ctx, service, recorder, index, input)
	return observed, err
}

func captureCandidateWithCalibration(
	ctx context.Context,
	service *usermemory.Service,
	recorder *Recorder,
	index FixtureIndex,
	input RuntimeCase,
) (memoryeval.CaseObservation, CandidateCalibrationTrace, error) {
	if service == nil || recorder == nil || !validRuntimeCase(input) {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, ErrCaptureInvalid
	}
	if err := recorder.Begin(input.AssistantMessageID); err != nil {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, err
	}
	finished := false
	defer func() {
		if !finished {
			recorder.Abort()
		}
	}()
	ctx = auth.WithUser(ctx, auth.User{ID: input.UserID})
	started := time.Now()
	_, summary, err := service.SearchRelevantWithHybridShadow(
		ctx,
		input.Query,
		input.ConversationID,
		input.AssistantMessageID,
		usermemory.MaxSearchResults,
	)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, errors.Join(ErrCaptureUnavailable, err)
	}
	transient, err := recorder.Finish(input.AssistantMessageID)
	if err != nil {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, err
	}
	finished = true

	candidateIDs := transient.candidates
	finalIDs := transient.final
	fallback := hybridFallback(summary, len(finalIDs))
	promptTokens := summary.EstimatedTokens
	if summary.Status != "completed" {
		if transient.candidates == nil {
			candidateIDs = []string{}
		} else {
			// Prepare already returned a SQL-authorized RRF surface, but an
			// uncompleted Record means its attempted final set did not pass the
			// production reauthorization boundary. Preserve candidates and
			// Provider egress while exposing no unauthorized final/injection.
			candidateIDs = append([]string(nil), transient.candidates...)
		}
		// v1 remains the real production prompt authority, but it is a separate
		// benchmark profile and must never be laundered into the v2 candidate.
		finalIDs = []string{}
		fallback = "no_memory"
		promptTokens = 0
	}
	opaqueCandidate, err := index.OpaqueMemoryIDs(candidateIDs)
	if err != nil {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, err
	}
	opaqueFinal, err := index.OpaqueMemoryIDs(finalIDs)
	if err != nil {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, err
	}
	opaqueProvider, err := index.OpaqueMemoryIDs(transient.providerSent)
	if err != nil {
		return memoryeval.CaseObservation{}, CandidateCalibrationTrace{}, err
	}
	hardCutoff := latency >= 2000 || summary.ResultCode == "HARD_CUTOFF" ||
		summary.FallbackCode == "HARD_CUTOFF"
	observed := memoryeval.CaseObservation{
		CaseID: input.CaseID, CandidateMemoryIDs: opaqueCandidate,
		FinalMemoryIDs:     opaqueFinal,
		InjectedMemoryIDs:  append([]string(nil), opaqueFinal...),
		PersistedMemoryIDs: []string{}, ProviderSentMemoryIDs: opaqueProvider,
		LatencyMilliseconds: latency, PromptMemoryTokens: promptTokens,
		HardCutoffApplied: hardCutoff, Fallback: fallback,
	}
	trace := CandidateCalibrationTrace{
		CaseID:                              input.CaseID,
		PreparedReady:                       transient.candidates != nil,
		MemoryIntentMargin:                  transient.memoryIntentMargin,
		MemoryIntentReady:                   transient.memoryIntentReady,
		AdmissionSimilarity:                 transient.admissionSimilarity,
		AdmissionReady:                      transient.admissionReady,
		RerankReady:                         transient.rerankReady,
		CloudJudgeReady:                     transient.cloudJudgeReady,
		CloudJudgeInputTokenUpperBound:      transient.cloudJudgeInputTokenUpperBound,
		MemoryToolRouteReady:                transient.memoryToolRouteReady,
		MemoryToolRouteUsed:                 transient.memoryToolRouteUsed,
		MemoryToolRouteInputTokenUpperBound: transient.memoryToolRouteInputTokenUpperBound,
		AbstentionCode:                      summary.FallbackCode,
		ResultCode:                          summary.ResultCode,
		FullObservation:                     observed,
		FinalRelevanceScores:                make([]float64, len(transient.final)),
	}
	for position, memoryID := range transient.final {
		score, ok := transient.rerankScores[memoryID]
		if !ok {
			trace.RerankReady = false
			continue
		}
		trace.FinalRelevanceScores[position] = score
	}
	return observed, trace, nil
}

func hybridFallback(summary usermemory.HybridShadowSummary, finalCount int) string {
	if finalCount == 0 {
		return "no_memory"
	}
	if summary.FallbackCode != "" && summary.FallbackCode != "NONE" {
		return "exact_bm25"
	}
	return "none"
}

func validRuntimeCase(value RuntimeCase) bool {
	return strings.TrimSpace(value.CaseID) != "" && strings.TrimSpace(value.Query) != "" &&
		strings.TrimSpace(value.UserID) != "" && strings.TrimSpace(value.ConversationID) != "" &&
		strings.TrimSpace(value.AssistantMessageID) != ""
}
