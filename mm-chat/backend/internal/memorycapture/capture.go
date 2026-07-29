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
	if service == nil || recorder == nil || !validRuntimeCase(input) {
		return memoryeval.CaseObservation{}, ErrCaptureInvalid
	}
	if err := recorder.Begin(input.AssistantMessageID); err != nil {
		return memoryeval.CaseObservation{}, err
	}
	finished := false
	defer func() {
		if !finished {
			recorder.Abort()
		}
	}()
	ctx = auth.WithUser(ctx, auth.User{ID: input.UserID})
	started := time.Now()
	baseline, summary, err := service.SearchRelevantWithHybridShadow(
		ctx,
		input.Query,
		input.ConversationID,
		input.AssistantMessageID,
		usermemory.MaxSearchResults,
	)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return memoryeval.CaseObservation{}, errors.Join(ErrCaptureUnavailable, err)
	}
	transient, err := recorder.Finish(input.AssistantMessageID)
	if err != nil {
		return memoryeval.CaseObservation{}, err
	}
	finished = true

	candidateIDs := transient.candidates
	finalIDs := transient.final
	fallback := hybridFallback(summary, len(finalIDs))
	promptTokens := summary.EstimatedTokens
	if summary.Status != "completed" {
		if transient.candidates == nil {
			candidateIDs = make([]string, len(baseline))
			for position, item := range baseline {
				candidateIDs[position] = item.ID
			}
			finalIDs = append([]string(nil), candidateIDs...)
		} else {
			// Prepare already returned a SQL-authorized RRF surface, but an
			// uncompleted Record means its attempted final set did not pass the
			// production reauthorization boundary. Preserve candidates and
			// Provider egress while exposing no unauthorized final/injection.
			candidateIDs = append([]string(nil), transient.candidates...)
			finalIDs = []string{}
		}
		fallback = "lexical_v1"
		promptTokens = usermemory.EstimatePromptMemoryTokens(baseline)
	}
	opaqueCandidate, err := index.OpaqueMemoryIDs(candidateIDs)
	if err != nil {
		return memoryeval.CaseObservation{}, err
	}
	opaqueFinal, err := index.OpaqueMemoryIDs(finalIDs)
	if err != nil {
		return memoryeval.CaseObservation{}, err
	}
	opaqueProvider, err := index.OpaqueMemoryIDs(transient.providerSent)
	if err != nil {
		return memoryeval.CaseObservation{}, err
	}
	hardCutoff := latency >= 2000 || summary.ResultCode == "HARD_CUTOFF" ||
		summary.FallbackCode == "HARD_CUTOFF"
	return memoryeval.CaseObservation{
		CaseID: input.CaseID, CandidateMemoryIDs: opaqueCandidate,
		FinalMemoryIDs:     opaqueFinal,
		InjectedMemoryIDs:  append([]string(nil), opaqueFinal...),
		PersistedMemoryIDs: []string{}, ProviderSentMemoryIDs: opaqueProvider,
		LatencyMilliseconds: latency, PromptMemoryTokens: promptTokens,
		HardCutoffApplied: hardCutoff, Fallback: fallback,
	}, nil
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
