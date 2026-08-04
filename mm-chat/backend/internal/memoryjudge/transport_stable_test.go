package memoryjudge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestTransportStableCandidateJudgeRetriesTypedFailuresWithExactFallbacks(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeValidJudgeStream(w)
	}))
	defer server.Close()

	adapter := newTransportStableTestAdapter(t, server.URL)
	waits := make([]time.Duration, 0, 2)
	judge, err := newTransportStableCandidateJudge(
		adapter,
		func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := judge.JudgeHybridCandidates(
		context.Background(),
		transportStableTestInput(),
	)
	if err != nil || requests != 3 ||
		len(waits) != 2 || waits[0] != TransportStableFirstRetryDelay ||
		waits[1] != TransportStableSecondRetryDelay ||
		result.ModelID != usermemory.HybridFixedMemoryJudgeModelID {
		t.Fatalf("result=%#v requests=%d waits=%v err=%v", result, requests, waits, err)
	}
}

func TestTransportStableCandidateJudgeHonorsExplicitRetryAfter(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeValidJudgeStream(w)
	}))
	defer server.Close()

	adapter := newTransportStableTestAdapter(t, server.URL)
	waits := make([]time.Duration, 0, 1)
	judge, err := newTransportStableCandidateJudge(
		adapter,
		func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := judge.JudgeHybridCandidates(
		context.Background(),
		transportStableTestInput(),
	); err != nil || requests != 2 || len(waits) != 1 || waits[0] != 7*time.Second {
		t.Fatalf("requests=%d waits=%v err=%v", requests, waits, err)
	}
}

func TestTransportStableCandidateJudgeDoesNotRetryDeterministicFailure(t *testing.T) {
	delegate := &transportStableFailureJudge{err: errors.New("private invalid output")}
	waits := 0
	judge, err := newTransportStableCandidateJudge(
		delegate,
		func(context.Context, time.Duration) error {
			waits++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := judge.JudgeHybridCandidates(
		context.Background(),
		transportStableTestInput(),
	); err == nil || delegate.calls != 1 || waits != 0 {
		t.Fatalf("calls=%d waits=%d err=%v", delegate.calls, waits, err)
	}
}

func newTransportStableTestAdapter(t *testing.T, baseURL string) *ChatAdapter {
	t.Helper()
	provider, err := chat.NewOpenAICompatibleProvider(chat.OpenAICompatibleProviderConfig{
		BaseURL:    baseURL,
		APIKey:     "example-fixture-judge-credential",
		ProviderID: fixedTransportStableTestProviderID,
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewChatAdapter(provider, chat.ModelRef{
		ProviderID: fixedTransportStableTestProviderID,
		ModelID:    usermemory.HybridFixedMemoryJudgeModelID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

const fixedTransportStableTestProviderID = "fixture-fixed-judge"

func transportStableTestInput() usermemory.HybridCandidateJudgeInput {
	return usermemory.HybridCandidateJudgeInput{
		Query: "Which school?",
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "Northwestern Polytechnical University"},
		},
	}
}

func writeValidJudgeStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"schemaVersion\\\":\\\"neo-chat.memory-cloud-candidate-judge-output.v1\\\",\\\"selectedOrdinals\\\":[0]}\"}}]}\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

type transportStableFailureJudge struct {
	calls int
	err   error
}

func (judge *transportStableFailureJudge) JudgeHybridCandidates(
	context.Context,
	usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	judge.calls++
	return usermemory.HybridCandidateJudgeResult{}, judge.err
}
