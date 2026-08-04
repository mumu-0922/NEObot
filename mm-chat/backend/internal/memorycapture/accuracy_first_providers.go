package memorycapture

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	AccuracyFirstRetryFallbackDelay = 5 * time.Second
	TransportStableSecondRetryDelay = 10 * time.Second
	AccuracyFirstInterCaseCooldown  = 1 * time.Second
)

func AccuracyFirstDevelopmentExecutionPolicy(
	providerMode string,
) (AccuracyFirstExecutionPolicy, error) {
	cooldownClock := ""
	switch providerMode {
	case ProviderModeFakeProtocol:
		cooldownClock = AccuracyFirstCooldownVirtualProtocolV1
	case ProviderModeLiveSiliconFlow:
		cooldownClock = AccuracyFirstCooldownWallClockV1
	default:
		return AccuracyFirstExecutionPolicy{}, ErrCaptureInvalid
	}
	return AccuracyFirstExecutionPolicy{
		SequenceVersion:                  AccuracyFirstExecutionSequenceV1,
		GlobalProviderRequestConcurrency: 1,
		ApplicationDeadlineMode:          memoryeval.MemoryJudgeApplicationDeadlineNoneV1,
		ProviderElapsedTimeoutMode:       memoryeval.MemoryJudgeApplicationDeadlineNoneV1,
		LatencyEvaluationMode:            memoryeval.MemoryJudgeLatencyDiagnosticOnlyV1,
		InterCaseCooldownMilliseconds:    int(AccuracyFirstInterCaseCooldown / time.Millisecond),
		InterCaseCooldownClock:           cooldownClock,
		RetryPolicyVersion:               AccuracyFirstRetryPolicyV1,
		MaximumRetriesPerProviderRequest: 1,
		RetryFallbackDelayMilliseconds:   int(AccuracyFirstRetryFallbackDelay / time.Millisecond),
	}, nil
}

// TransportStableDevelopmentExecutionPolicy preserves every schema-v12
// execution boundary except the Judge-specific retry ceiling. Retrieval
// Provider calls remain limited to one retry.
func TransportStableDevelopmentExecutionPolicy(
	providerMode string,
) (AccuracyFirstExecutionPolicy, error) {
	policy, err := AccuracyFirstDevelopmentExecutionPolicy(providerMode)
	if err != nil {
		return AccuracyFirstExecutionPolicy{}, err
	}
	policy.SequenceVersion = TransportStableExecutionSequenceV2
	policy.RetryPolicyVersion = TransportStableRetryPolicyV2
	policy.MaximumJudgeRetriesPerRequest = 2
	policy.SecondJudgeRetryDelayMilliseconds = int(
		TransportStableSecondRetryDelay / time.Millisecond,
	)
	return policy, nil
}

type accuracyFirstWait func(context.Context, time.Duration) error

// AccuracyFirstProviderController owns one global request gate for projection
// embedding, query embedding, rerank, and judge work. The schema-v12 runner
// therefore cannot place two Provider requests in flight even if a caller
// accidentally reintroduces goroutines around one of the wrapped interfaces.
type AccuracyFirstProviderController struct {
	requestGate             sync.Mutex
	mu                      sync.Mutex
	wait                    accuracyFirstWait
	virtualTime             bool
	judgeFailureDiagnostics bool
	maximumJudgeRetries     int
	telemetry               AccuracyFirstProviderTelemetry
	latencies               map[string][]int64
}

type accuracyFirstPassageEmbedder struct {
	controller *AccuracyFirstProviderController
	delegate   PassageEmbedder
}

type accuracyFirstHybridProvider struct {
	controller *AccuracyFirstProviderController
	delegate   usermemory.HybridShadowProvider
}

type accuracyFirstCandidateJudge struct {
	controller *AccuracyFirstProviderController
	delegate   usermemory.HybridCandidateJudge
}

func WrapAccuracyFirstDevelopmentProviders(
	providerMode string,
	passage PassageEmbedder,
	hybrid usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
) (
	PassageEmbedder,
	usermemory.HybridShadowProvider,
	usermemory.HybridCandidateJudge,
	*AccuracyFirstProviderController,
	error,
) {
	wait := waitAccuracyFirst
	virtualTime := false
	switch providerMode {
	case ProviderModeFakeProtocol:
		virtualTime = true
		wait = func(context.Context, time.Duration) error { return nil }
	case ProviderModeLiveSiliconFlow:
	default:
		return nil, nil, nil, nil, ErrCaptureInvalid
	}
	return wrapAccuracyFirstDevelopmentProviders(
		passage,
		hybrid,
		judge,
		wait,
		virtualTime,
		false,
	)
}

// WrapAccuracyFirstJudgeFailureDiagnosticDevelopmentProviders preserves the
// schema-v12 serial execution behavior while enabling schema-v13-only bounded
// Judge attempt failure aggregation.
func WrapAccuracyFirstJudgeFailureDiagnosticDevelopmentProviders(
	providerMode string,
	passage PassageEmbedder,
	hybrid usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
) (
	PassageEmbedder,
	usermemory.HybridShadowProvider,
	usermemory.HybridCandidateJudge,
	*AccuracyFirstProviderController,
	error,
) {
	wait := waitAccuracyFirst
	virtualTime := false
	switch providerMode {
	case ProviderModeFakeProtocol:
		virtualTime = true
		wait = func(context.Context, time.Duration) error { return nil }
	case ProviderModeLiveSiliconFlow:
	default:
		return nil, nil, nil, nil, ErrCaptureInvalid
	}
	return wrapAccuracyFirstDevelopmentProviders(
		passage,
		hybrid,
		judge,
		wait,
		virtualTime,
		true,
	)
}

// WrapTransportStableMemoryJudgeDevelopmentProviders keeps the schema-v13
// aggregate failure diagnostics and serial gate while permitting one extra
// Judge-only transient retry. BGE retry behavior remains unchanged.
func WrapTransportStableMemoryJudgeDevelopmentProviders(
	providerMode string,
	passage PassageEmbedder,
	hybrid usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
) (
	PassageEmbedder,
	usermemory.HybridShadowProvider,
	usermemory.HybridCandidateJudge,
	*AccuracyFirstProviderController,
	error,
) {
	wait := waitAccuracyFirst
	virtualTime := false
	switch providerMode {
	case ProviderModeFakeProtocol:
		virtualTime = true
		wait = func(context.Context, time.Duration) error { return nil }
	case ProviderModeLiveSiliconFlow:
	default:
		return nil, nil, nil, nil, ErrCaptureInvalid
	}
	passageProvider, hybridProvider, candidateJudge, controller, err :=
		wrapAccuracyFirstDevelopmentProviders(
			passage,
			hybrid,
			judge,
			wait,
			virtualTime,
			true,
		)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	controller.maximumJudgeRetries = 2
	return passageProvider, hybridProvider, candidateJudge, controller, nil
}

func wrapAccuracyFirstDevelopmentProviders(
	passage PassageEmbedder,
	hybrid usermemory.HybridShadowProvider,
	judge usermemory.HybridCandidateJudge,
	wait accuracyFirstWait,
	virtualTime bool,
	judgeFailureDiagnostics bool,
) (
	PassageEmbedder,
	usermemory.HybridShadowProvider,
	usermemory.HybridCandidateJudge,
	*AccuracyFirstProviderController,
	error,
) {
	if passage == nil || hybrid == nil || judge == nil || wait == nil {
		return nil, nil, nil, nil, ErrCaptureInvalid
	}
	controller := &AccuracyFirstProviderController{
		wait:                    wait,
		virtualTime:             virtualTime,
		judgeFailureDiagnostics: judgeFailureDiagnostics,
		latencies:               make(map[string][]int64),
	}
	if controller.judgeFailureDiagnostics {
		controller.telemetry.JudgeAttemptFailureCategoryCounts = make(map[string]int)
	}
	return &accuracyFirstPassageEmbedder{controller: controller, delegate: passage},
		&accuracyFirstHybridProvider{controller: controller, delegate: hybrid},
		&accuracyFirstCandidateJudge{controller: controller, delegate: judge},
		controller,
		nil
}

func (controller *AccuracyFirstProviderController) Snapshot() AccuracyFirstProviderTelemetry {
	if controller == nil {
		return AccuracyFirstProviderTelemetry{}
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	telemetry := controller.telemetry
	telemetry.PassageEmbeddingLatency = accuracyFirstLatencyDiagnostics(
		controller.latencies["passage_embedding"],
	)
	telemetry.QueryEmbeddingLatency = accuracyFirstLatencyDiagnostics(
		controller.latencies["query_embedding"],
	)
	telemetry.RerankLatency = accuracyFirstLatencyDiagnostics(
		controller.latencies["rerank"],
	)
	telemetry.JudgeLatency = accuracyFirstLatencyDiagnostics(
		controller.latencies["judge"],
	)
	if controller.telemetry.JudgeAttemptFailureCategoryCounts != nil {
		telemetry.JudgeAttemptFailureCategoryCounts = make(map[string]int,
			len(controller.telemetry.JudgeAttemptFailureCategoryCounts))
		for category, count := range controller.telemetry.JudgeAttemptFailureCategoryCounts {
			telemetry.JudgeAttemptFailureCategoryCounts[category] = count
		}
	}
	return telemetry
}

func (controller *AccuracyFirstProviderController) waitInterCaseCooldown(
	ctx context.Context,
) error {
	if controller == nil || controller.wait == nil {
		return ErrCaptureInvalid
	}
	controller.requestGate.Lock()
	defer controller.requestGate.Unlock()
	started := time.Now()
	if err := controller.wait(ctx, AccuracyFirstInterCaseCooldown); err != nil {
		return err
	}
	elapsed := time.Since(started).Milliseconds()
	controller.mu.Lock()
	controller.telemetry.InterCaseCooldownCount++
	controller.telemetry.InterCaseCooldownMilliseconds +=
		int(AccuracyFirstInterCaseCooldown / time.Millisecond)
	if !controller.virtualTime {
		controller.telemetry.InterCaseCooldownElapsedMillis += elapsed
	}
	controller.mu.Unlock()
	return nil
}

func (embedder *accuracyFirstPassageEmbedder) EmbedSiliconFlowPassages(
	ctx context.Context,
	request ragproviders.PassageEmbeddingRequest,
) (ragproviders.PassageEmbeddingResponse, error) {
	if embedder == nil || embedder.controller == nil || embedder.delegate == nil {
		return ragproviders.PassageEmbeddingResponse{}, ErrCaptureInvalid
	}
	embedder.controller.requestGate.Lock()
	defer embedder.controller.requestGate.Unlock()
	return executeAccuracyFirstRequest(
		ctx,
		embedder.controller,
		"passage_embedding",
		func() (ragproviders.PassageEmbeddingResponse, error) {
			return embedder.delegate.EmbedSiliconFlowPassages(ctx, request)
		},
		ragproviders.ProviderRetryDelay,
		0,
	)
}

func (provider *accuracyFirstHybridProvider) EmbedQuery(
	ctx context.Context,
	query string,
) (ragproviders.QueryEmbedding, error) {
	if provider == nil || provider.controller == nil || provider.delegate == nil {
		return ragproviders.QueryEmbedding{}, ErrCaptureInvalid
	}
	provider.controller.requestGate.Lock()
	defer provider.controller.requestGate.Unlock()
	return executeAccuracyFirstRequest(
		ctx,
		provider.controller,
		"query_embedding",
		func() (ragproviders.QueryEmbedding, error) {
			return provider.delegate.EmbedQuery(ctx, query)
		},
		ragproviders.ProviderRetryDelay,
		0,
	)
}

func (provider *accuracyFirstHybridProvider) Rerank(
	ctx context.Context,
	query string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	if provider == nil || provider.controller == nil || provider.delegate == nil {
		return nil, ErrCaptureInvalid
	}
	provider.controller.requestGate.Lock()
	defer provider.controller.requestGate.Unlock()
	return executeAccuracyFirstRequest(
		ctx,
		provider.controller,
		"rerank",
		func() ([]ragproviders.RerankResult, error) {
			return provider.delegate.Rerank(ctx, query, documents)
		},
		ragproviders.ProviderRetryDelay,
		0,
	)
}

func (judge *accuracyFirstCandidateJudge) JudgeHybridCandidates(
	ctx context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	if judge == nil || judge.controller == nil || judge.delegate == nil {
		return usermemory.HybridCandidateJudgeResult{}, ErrCaptureInvalid
	}
	inputTokenUpperBound, err := cloudJudgeInputTokenUpperBound(input)
	if err != nil {
		return usermemory.HybridCandidateJudgeResult{}, ErrCaptureInvalid
	}
	judge.controller.requestGate.Lock()
	defer judge.controller.requestGate.Unlock()
	return executeAccuracyFirstRequest(
		ctx,
		judge.controller,
		"judge",
		func() (usermemory.HybridCandidateJudgeResult, error) {
			return judge.delegate.JudgeHybridCandidates(ctx, input)
		},
		chat.ProviderRetryDelay,
		inputTokenUpperBound,
	)
}

func executeAccuracyFirstRequest[T any](
	ctx context.Context,
	controller *AccuracyFirstProviderController,
	operation string,
	request func() (T, error),
	retryAdvice func(error) (time.Duration, bool),
	judgeInputTokenUpperBound int,
) (T, error) {
	var zero T
	if controller == nil || request == nil || retryAdvice == nil {
		return zero, ErrCaptureInvalid
	}
	maximumRetries := 1
	if operation == "judge" && controller.maximumJudgeRetries > 0 {
		maximumRetries = controller.maximumJudgeRetries
	}
	for attempt := 0; attempt <= maximumRetries; attempt++ {
		controller.recordAttempt(operation, attempt > 0, judgeInputTokenUpperBound)
		started := time.Now()
		result, err := request()
		controller.recordRequestLatency(operation, time.Since(started))
		if err == nil {
			return result, nil
		}
		controller.recordFailure(operation, err)
		if attempt >= maximumRetries || ctx.Err() != nil {
			return zero, err
		}
		delay, retryable := retryAdvice(err)
		if !retryable {
			return zero, err
		}
		if operation == "judge" && maximumRetries > 1 {
			if explicit, ok := chat.ProviderExplicitRetryDelay(err); ok {
				delay = explicit
			} else {
				delay = accuracyFirstFallbackDelay(operation, attempt+1)
			}
		} else if delay < 0 {
			delay = accuracyFirstFallbackDelay(operation, attempt+1)
		}
		if err := controller.wait(ctx, delay); err != nil {
			return zero, errors.Join(err, ctx.Err())
		}
	}
	return zero, ErrCaptureUnavailable
}

func accuracyFirstFallbackDelay(operation string, retryNumber int) time.Duration {
	if operation == "judge" && retryNumber == 2 {
		return TransportStableSecondRetryDelay
	}
	return AccuracyFirstRetryFallbackDelay
}

func (controller *AccuracyFirstProviderController) recordFailure(
	operation string,
	err error,
) {
	if controller == nil || !controller.judgeFailureDiagnostics || operation != "judge" {
		return
	}
	category := memoryjudge.FailureCategory(err)
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.telemetry.JudgeAttemptFailureCategoryCounts == nil {
		controller.telemetry.JudgeAttemptFailureCategoryCounts = make(map[string]int)
	}
	controller.telemetry.JudgeAttemptFailureCategoryCounts[category]++
}

func (controller *AccuracyFirstProviderController) recordRequestLatency(
	operation string,
	elapsed time.Duration,
) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if controller.latencies == nil {
		controller.latencies = make(map[string][]int64)
	}
	controller.latencies[operation] = append(
		controller.latencies[operation],
		elapsed.Milliseconds(),
	)
}

func accuracyFirstLatencyDiagnostics(
	values []int64,
) AccuracyFirstLatencyDiagnostics {
	if len(values) == 0 {
		return AccuracyFirstLatencyDiagnostics{}
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var total int64
	for _, value := range ordered {
		total += value
	}
	return AccuracyFirstLatencyDiagnostics{
		SampleCount:                len(ordered),
		TotalMilliseconds:          total,
		P95LatencyMilliseconds:     accuracyFirstPercentile(ordered, 95),
		P99LatencyMilliseconds:     accuracyFirstPercentile(ordered, 99),
		MaximumLatencyMilliseconds: ordered[len(ordered)-1],
	}
}

func accuracyFirstPercentile(ordered []int64, percentile int) int64 {
	if len(ordered) == 0 || percentile < 1 || percentile > 100 {
		return 0
	}
	index := (len(ordered)*percentile + 99) / 100
	return ordered[index-1]
}

func (controller *AccuracyFirstProviderController) recordAttempt(
	operation string,
	retry bool,
	judgeInputTokenUpperBound int,
) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	switch operation {
	case "passage_embedding":
		controller.telemetry.PassageEmbeddingAttempts++
		if retry {
			controller.telemetry.PassageEmbeddingRetries++
		}
	case "query_embedding":
		controller.telemetry.QueryEmbeddingAttempts++
		if retry {
			controller.telemetry.QueryEmbeddingRetries++
		}
	case "rerank":
		controller.telemetry.RerankAttempts++
		if retry {
			controller.telemetry.RerankRetries++
		}
	case "judge":
		controller.telemetry.JudgeAttempts++
		controller.telemetry.JudgeInputTokenUpperBound += judgeInputTokenUpperBound
		if retry {
			controller.telemetry.JudgeRetries++
			controller.telemetry.JudgeRetryInputTokenUpperBound +=
				judgeInputTokenUpperBound
		}
	}
}

func waitAccuracyFirst(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var (
	_ PassageEmbedder                 = (*accuracyFirstPassageEmbedder)(nil)
	_ usermemory.HybridShadowProvider = (*accuracyFirstHybridProvider)(nil)
	_ usermemory.HybridCandidateJudge = (*accuracyFirstCandidateJudge)(nil)
)
