package memorycapture

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
)

func TestExecuteAccuracyFirstRequestRetriesOnceAndRecordsWait(t *testing.T) {
	delays := make([]time.Duration, 0, 1)
	controller := &AccuracyFirstProviderController{
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	}
	attempts := 0
	result, err := executeAccuracyFirstRequest(
		context.Background(),
		controller,
		"judge",
		func() (string, error) {
			attempts++
			if attempts == 1 {
				return "", errors.New("transient")
			}
			return "ok", nil
		},
		func(error) (time.Duration, bool) { return 7 * time.Second, true },
		123,
	)
	telemetry := controller.Snapshot()
	if err != nil || result != "ok" || attempts != 2 ||
		len(delays) != 1 || delays[0] != 7*time.Second ||
		telemetry.JudgeAttempts != 2 || telemetry.JudgeRetries != 1 ||
		telemetry.JudgeInputTokenUpperBound != 246 ||
		telemetry.JudgeRetryInputTokenUpperBound != 123 ||
		telemetry.JudgeLatency.SampleCount != 2 {
		t.Fatalf(
			"result=%q err=%v attempts=%d delays=%v telemetry=%#v",
			result,
			err,
			attempts,
			delays,
			telemetry,
		)
	}
}

func TestJudgeFailureDiagnosticControllerCountsRecoveredAndExhaustedAttempts(t *testing.T) {
	tests := []struct {
		name      string
		succeed   bool
		wantCount int
	}{
		{name: "retry recovered", succeed: true, wantCount: 1},
		{name: "retry exhausted", succeed: false, wantCount: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := &AccuracyFirstProviderController{
				wait:                    func(context.Context, time.Duration) error { return nil },
				judgeFailureDiagnostics: true,
				telemetry: AccuracyFirstProviderTelemetry{
					JudgeAttemptFailureCategoryCounts: make(map[string]int),
				},
			}
			attempts := 0
			result, err := executeAccuracyFirstRequest(
				context.Background(),
				controller,
				"judge",
				func() (string, error) {
					attempts++
					if test.succeed && attempts == 2 {
						return "ok", nil
					}
					return "", memoryjudge.NewFailure(
						string(chat.ProviderFailureRateLimited),
						errors.New("private upstream body"),
					)
				},
				func(error) (time.Duration, bool) { return 0, true },
				100,
			)
			if test.succeed && (err != nil || result != "ok") {
				t.Fatalf("recovered result=%q err=%v", result, err)
			}
			if !test.succeed && err == nil {
				t.Fatal("exhausted retry succeeded")
			}
			telemetry := controller.Snapshot()
			category := string(chat.ProviderFailureRateLimited)
			failureCount := telemetry.JudgeAttemptFailureCategoryCounts[category]
			if failureCount != test.wantCount || telemetry.JudgeAttempts != 2 ||
				telemetry.JudgeRetries != 1 {
				t.Fatalf("telemetry=%#v", telemetry)
			}
		})
	}
}

func TestTransportStableJudgeRetriesTwiceWithVersionedFallbacks(t *testing.T) {
	delays := make([]time.Duration, 0, 2)
	controller := &AccuracyFirstProviderController{
		wait: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
		judgeFailureDiagnostics: true,
		maximumJudgeRetries:     2,
		telemetry: AccuracyFirstProviderTelemetry{
			JudgeAttemptFailureCategoryCounts: make(map[string]int),
		},
	}
	attempts := 0
	result, err := executeAccuracyFirstRequest(
		context.Background(),
		controller,
		"judge",
		func() (string, error) {
			attempts++
			if attempts == 3 {
				return "ok", nil
			}
			return "", memoryjudge.NewFailure(
				string(chat.ProviderFailureTransportFailed),
				errors.New("private transport error"),
			)
		},
		func(error) (time.Duration, bool) { return 5 * time.Second, true },
		100,
	)
	telemetry := controller.Snapshot()
	if err != nil || result != "ok" || attempts != 3 ||
		len(delays) != 2 || delays[0] != 5*time.Second ||
		delays[1] != 10*time.Second || telemetry.JudgeAttempts != 3 ||
		telemetry.JudgeRetries != 2 || telemetry.JudgeRetryInputTokenUpperBound != 200 ||
		telemetry.JudgeAttemptFailureCategoryCounts[string(chat.ProviderFailureTransportFailed)] != 2 {
		t.Fatalf(
			"result=%q err=%v attempts=%d delays=%v telemetry=%#v",
			result,
			err,
			attempts,
			delays,
			telemetry,
		)
	}
}

func TestAccuracyFirstV12ControllerOmitsJudgeFailureDiagnostics(t *testing.T) {
	controller := &AccuracyFirstProviderController{
		wait: func(context.Context, time.Duration) error { return nil },
	}
	_, _ = executeAccuracyFirstRequest(
		context.Background(), controller, "judge",
		func() (struct{}, error) { return struct{}{}, errors.New("private") },
		func(error) (time.Duration, bool) { return 0, false },
		100,
	)
	if controller.Snapshot().JudgeAttemptFailureCategoryCounts != nil {
		t.Fatal("schema-v12 controller recorded schema-v13 diagnostics")
	}
}

func TestAccuracyFirstInterCaseCooldownUsesTheControllerGateAndVirtualClock(t *testing.T) {
	waits := make([]time.Duration, 0, 1)
	controller := &AccuracyFirstProviderController{
		wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
		virtualTime: true,
		latencies:   make(map[string][]int64),
	}
	if err := controller.waitInterCaseCooldown(context.Background()); err != nil {
		t.Fatal(err)
	}
	telemetry := controller.Snapshot()
	if len(waits) != 1 || waits[0] != time.Second ||
		telemetry.InterCaseCooldownCount != 1 ||
		telemetry.InterCaseCooldownMilliseconds != 1000 ||
		telemetry.InterCaseCooldownElapsedMillis != 0 {
		t.Fatalf("cooldown waits=%v telemetry=%#v", waits, telemetry)
	}
}

func TestExecuteAccuracyFirstRequestDoesNotRetryDeterministicFailure(t *testing.T) {
	controller := &AccuracyFirstProviderController{
		wait: func(context.Context, time.Duration) error {
			t.Fatal("non-retryable failure waited")
			return nil
		},
	}
	attempts := 0
	_, err := executeAccuracyFirstRequest(
		context.Background(),
		controller,
		"rerank",
		func() (struct{}, error) {
			attempts++
			return struct{}{}, errors.New("invalid response")
		},
		func(error) (time.Duration, bool) { return 0, false },
		0,
	)
	telemetry := controller.Snapshot()
	if err == nil || attempts != 1 || telemetry.RerankAttempts != 1 ||
		telemetry.RerankRetries != 0 {
		t.Fatalf("err=%v attempts=%d telemetry=%#v", err, attempts, telemetry)
	}
}

func TestAccuracyFirstControllerSerializesDifferentProviderKinds(t *testing.T) {
	controller := &AccuracyFirstProviderController{wait: waitAccuracyFirst}
	tracker := &accuracyFirstConcurrencyTracker{}
	start := make(chan struct{})
	var group sync.WaitGroup
	for _, operation := range []string{"query_embedding", "judge"} {
		operation := operation
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			controller.requestGate.Lock()
			defer controller.requestGate.Unlock()
			tracker.enter()
			time.Sleep(10 * time.Millisecond)
			tracker.leave()
			controller.recordAttempt(operation, false, 0)
		}()
	}
	close(start)
	group.Wait()
	telemetry := controller.Snapshot()
	if tracker.maximum != 1 || telemetry.QueryEmbeddingAttempts != 1 ||
		telemetry.JudgeAttempts != 1 {
		t.Fatalf("maximum=%d telemetry=%#v", tracker.maximum, telemetry)
	}
}

type accuracyFirstConcurrencyTracker struct {
	mu      sync.Mutex
	active  int
	maximum int
}

func (tracker *accuracyFirstConcurrencyTracker) enter() {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.active++
	tracker.maximum = max(tracker.maximum, tracker.active)
}

func (tracker *accuracyFirstConcurrencyTracker) leave() {
	tracker.mu.Lock()
	tracker.active--
	tracker.mu.Unlock()
}
