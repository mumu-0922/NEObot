package ragevalcapture

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	SupplementalNoAnswerLatencyDiagnosticVersion = "neo-chat.rag-supplemental-no-answer-latency-diagnostic.v1"
	supplementalLatencyPhaseCases                = 4
	supplementalLatencySelectionPolicy           = "first-two-chinese-cases-across-pdf-docx-pptx-xlsx-v1"
	supplementalLatencyFailure                   = "P95 retrieval latency exceeded the budget"
)

var supplementalLatencyFormats = [...]string{"pdf", "docx", "pptx", "xlsx"}

type SupplementalNoAnswerLatencyDiagnosticInput struct {
	SupplementalNoAnswerInput
	SourceReport LoadedSupplementalNoAnswerReport
}

type SupplementalNoAnswerLatencyDiagnostic struct {
	SchemaVersion             string                            `json:"schemaVersion"`
	CaptureVersion            string                            `json:"captureVersion"`
	PromotionEvidence         bool                              `json:"promotionEvidence"`
	DiagnosticIntegrityPassed bool                              `json:"diagnosticIntegrityPassed"`
	Conclusion                string                            `json:"conclusion"`
	CapturedAt                string                            `json:"capturedAt"`
	Suite                     SupplementalNoAnswerSuiteSummary  `json:"suite"`
	SourceReport              SupplementalLatencySourceReport   `json:"sourceReport"`
	Candidate                 SupplementalNoAnswerCandidate     `json:"candidate"`
	Configuration             SupplementalNoAnswerConfiguration `json:"configuration"`
	Criteria                  SupplementalNoAnswerCriteria      `json:"criteria"`
	SelectionPolicy           string                            `json:"selectionPolicy"`
	Cold                      SupplementalNoAnswerLatencyPhase  `json:"cold"`
	Warm                      SupplementalNoAnswerLatencyPhase  `json:"warm"`
	Delta                     SupplementalNoAnswerLatencyDelta  `json:"delta"`
	Failures                  []string                          `json:"failures"`
}

type SupplementalLatencySourceReport struct {
	RawSHA256              string `json:"rawSha256"`
	CapturedAt             string `json:"capturedAt"`
	P50LatencyMilliseconds int64  `json:"p50LatencyMilliseconds"`
	P95LatencyMilliseconds int64  `json:"p95LatencyMilliseconds"`
	MaximumLatencyMS       int64  `json:"maximumLatencyMilliseconds"`
}

type SupplementalNoAnswerLatencyPhase struct {
	Name    string                                  `json:"name"`
	CaseIDs []string                                `json:"caseIds"`
	Summary SupplementalNoAnswerSummary             `json:"summary"`
	Metrics SupplementalNoAnswerLatencyPhaseMetrics `json:"metrics"`
	Cases   []SupplementalNoAnswerObservation       `json:"cases"`
}

type SupplementalNoAnswerLatencyPhaseMetrics struct {
	P50LatencyMilliseconds int64                     `json:"p50LatencyMilliseconds"`
	P95LatencyMilliseconds int64                     `json:"p95LatencyMilliseconds"`
	MaximumLatencyMS       int64                     `json:"maximumLatencyMilliseconds"`
	P95Breakdown           PreflightLatencyBreakdown `json:"p95Breakdown"`
}

type SupplementalNoAnswerLatencyDelta struct {
	P95ReductionMilliseconds int64   `json:"p95ReductionMilliseconds"`
	P95ReductionRate         float64 `json:"p95ReductionRate"`
}

func CaptureSupplementalNoAnswerLatencyDiagnostic(
	ctx context.Context,
	input SupplementalNoAnswerLatencyDiagnosticInput,
) (SupplementalNoAnswerLatencyDiagnostic, error) {
	if err := validateSupplementalNoAnswerInput(input.SupplementalNoAnswerInput); err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	if input.Concurrency != supplementalLatencyPhaseCases {
		return SupplementalNoAnswerLatencyDiagnostic{}, errors.New(
			"supplemental latency diagnostic requires concurrency 4",
		)
	}
	status, err := input.Store.Status(ctx)
	if err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	if err := validateCaptureStatus(input.CaptureInput, status); err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	if err := validateSupplementalNoAnswerBinding(
		input.SupplementalNoAnswerInput,
		status,
	); err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	if err := validateSupplementalLatencySourceReport(input, status); err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	coldCases, warmCases, err := selectSupplementalLatencyCases(
		input.LoadedSuite.Suite.Cases,
	)
	if err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	cold, err := captureSupplementalLatencyPhase(ctx, input, "cold", coldCases)
	if err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	warm, err := captureSupplementalLatencyPhase(ctx, input, "warm", warmCases)
	if err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	failures := supplementalLatencyIntegrityFailures(cold, warm)
	conclusion := supplementalLatencyConclusion(
		len(failures) == 0,
		cold.Metrics.P95LatencyMilliseconds,
		warm.Metrics.P95LatencyMilliseconds,
		input.LoadedSuite.Suite.Criteria.MaximumP95LatencyMilliseconds,
	)
	sourceMetrics := supplementalLatencyMetrics(input.SourceReport.Report.Cases)
	diagnostic := SupplementalNoAnswerLatencyDiagnostic{
		SchemaVersion:             SupplementalNoAnswerLatencyDiagnosticVersion,
		CaptureVersion:            CaptureVersion,
		PromotionEvidence:         false,
		DiagnosticIntegrityPassed: len(failures) == 0,
		Conclusion:                conclusion,
		CapturedAt:                input.Clock().UTC().Format(time.RFC3339),
		Suite: SupplementalNoAnswerSuiteSummary{
			ID: input.LoadedSuite.Suite.ID, RawSHA256: input.LoadedSuite.RawSHA256,
			Cases: len(input.LoadedSuite.Suite.Cases),
		},
		SourceReport: SupplementalLatencySourceReport{
			RawSHA256:              input.SourceReport.RawSHA256,
			CapturedAt:             input.SourceReport.Report.CapturedAt,
			P50LatencyMilliseconds: sourceMetrics.P50LatencyMilliseconds,
			P95LatencyMilliseconds: sourceMetrics.P95LatencyMilliseconds,
			MaximumLatencyMS:       sourceMetrics.MaximumLatencyMS,
		},
		Candidate: SupplementalNoAnswerCandidate{
			GenerationID:         status.CandidateGenerationID,
			ArtifactManifestHash: status.CandidateArtifactManifestHash,
			ChunkProfileHash:     status.CandidateChunkProfileHash,
			Status:               status.CandidateStatus, Readiness: status.CandidateReadiness,
		},
		Configuration: SupplementalNoAnswerConfiguration{
			RetrievalProvider:        captureProviderConfiguration(input.CandidateProvider),
			AnswerProviderID:         input.AnswerProviderID,
			AnswerModelID:            input.AnswerModelID,
			Concurrency:              input.Concurrency,
			ScoringPolicy:            captureScoringPolicy,
			GenerationHeadRevision:   status.HeadRevision,
			CorpusProjectionRevision: status.CorpusProjectionRevision,
		},
		Criteria: input.LoadedSuite.Suite.Criteria, SelectionPolicy: supplementalLatencySelectionPolicy,
		Cold: cold, Warm: warm,
		Delta: SupplementalNoAnswerLatencyDelta{
			P95ReductionMilliseconds: cold.Metrics.P95LatencyMilliseconds -
				warm.Metrics.P95LatencyMilliseconds,
			P95ReductionRate: supplementalLatencyReductionRate(
				cold.Metrics.P95LatencyMilliseconds,
				warm.Metrics.P95LatencyMilliseconds,
			),
		},
		Failures: failures,
	}
	if err := validateSupplementalLatencyDiagnostic(diagnostic); err != nil {
		return SupplementalNoAnswerLatencyDiagnostic{}, err
	}
	return diagnostic, nil
}

func validateSupplementalLatencySourceReport(
	input SupplementalNoAnswerLatencyDiagnosticInput,
	status GenerationStatus,
) error {
	source := input.SourceReport
	report := source.Report
	if err := validateSupplementalNoAnswerReport(report); err != nil {
		return err
	}
	if !validCaptureHash(source.RawSHA256) ||
		report.Passed || len(report.Failures) == 0 ||
		report.Suite.ID != input.LoadedSuite.Suite.ID ||
		report.Suite.RawSHA256 != input.LoadedSuite.RawSHA256 ||
		report.Candidate.GenerationID != status.CandidateGenerationID ||
		report.Candidate.ArtifactManifestHash != status.CandidateArtifactManifestHash ||
		report.Candidate.ChunkProfileHash != status.CandidateChunkProfileHash ||
		report.Configuration.RetrievalProvider !=
			captureProviderConfiguration(input.CandidateProvider) ||
		report.Configuration.AnswerProviderID != input.AnswerProviderID ||
		report.Configuration.AnswerModelID != input.AnswerModelID ||
		report.Configuration.Concurrency != input.Concurrency ||
		report.Configuration.GenerationHeadRevision != status.HeadRevision ||
		report.Configuration.CorpusProjectionRevision != status.CorpusProjectionRevision ||
		!reflect.DeepEqual(report.Criteria, input.LoadedSuite.Suite.Criteria) ||
		report.Summary.P95LatencyMilliseconds <=
			report.Criteria.MaximumP95LatencyMilliseconds {
		return errors.New("supplemental latency source report binding is invalid")
	}
	for _, failure := range report.Failures {
		if !strings.HasSuffix(failure, supplementalLatencyFailure) {
			return errors.New("supplemental latency source report has non-latency failures")
		}
	}
	return nil
}

func selectSupplementalLatencyCases(
	cases []SupplementalNoAnswerCase,
) ([]SupplementalNoAnswerCase, []SupplementalNoAnswerCase, error) {
	cold := make([]SupplementalNoAnswerCase, 0, supplementalLatencyPhaseCases)
	warm := make([]SupplementalNoAnswerCase, 0, supplementalLatencyPhaseCases)
	for _, format := range supplementalLatencyFormats {
		matched := make([]SupplementalNoAnswerCase, 0, 2)
		for _, item := range cases {
			if item.Language == "chinese" && item.Format == format {
				matched = append(matched, item)
				if len(matched) == 2 {
					break
				}
			}
		}
		if len(matched) != 2 {
			return nil, nil, errors.New(
				"supplemental latency diagnostic case selection is incomplete",
			)
		}
		cold = append(cold, matched[0])
		warm = append(warm, matched[1])
	}
	return cold, warm, nil
}

func captureSupplementalLatencyPhase(
	ctx context.Context,
	input SupplementalNoAnswerLatencyDiagnosticInput,
	name string,
	cases []SupplementalNoAnswerCase,
) (SupplementalNoAnswerLatencyPhase, error) {
	observations, err := captureSupplementalNoAnswerCaseSet(
		ctx,
		input.SupplementalNoAnswerInput,
		cases,
	)
	if err != nil {
		return SupplementalNoAnswerLatencyPhase{}, err
	}
	caseIDs := make([]string, len(cases))
	for index, item := range cases {
		caseIDs[index] = item.ID
	}
	return SupplementalNoAnswerLatencyPhase{
		Name: name, CaseIDs: caseIDs,
		Summary: summarizeSupplementalNoAnswer(
			observations,
			input.LoadedSuite.Suite.Criteria,
			true,
		),
		Metrics: supplementalLatencyMetrics(observations),
		Cases:   observations,
	}, nil
}

func supplementalLatencyMetrics(
	observations []SupplementalNoAnswerObservation,
) SupplementalNoAnswerLatencyPhaseMetrics {
	latencies := make([]int64, len(observations))
	embed := make([]int64, len(observations))
	fetch := make([]int64, len(observations))
	hydrate := make([]int64, len(observations))
	rerank := make([]int64, len(observations))
	pipeline := make([]int64, len(observations))
	for index, item := range observations {
		latencies[index] = item.LatencyMilliseconds
		embed[index] = item.LatencyBreakdown.EmbedQueryMilliseconds
		fetch[index] = item.LatencyBreakdown.FetchCandidatesMilliseconds
		hydrate[index] = item.LatencyBreakdown.HydrateEvidenceMilliseconds
		rerank[index] = item.LatencyBreakdown.RerankMilliseconds
		pipeline[index] = item.LatencyBreakdown.PipelineTotalMilliseconds
	}
	return SupplementalNoAnswerLatencyPhaseMetrics{
		P50LatencyMilliseconds: supplementalLatencyPercentile(latencies, 0.50),
		P95LatencyMilliseconds: supplementalLatencyPercentile(latencies, 0.95),
		MaximumLatencyMS:       supplementalLatencyPercentile(latencies, 1),
		P95Breakdown: PreflightLatencyBreakdown{
			EmbedQueryMilliseconds:      supplementalLatencyPercentile(embed, 0.95),
			FetchCandidatesMilliseconds: supplementalLatencyPercentile(fetch, 0.95),
			HydrateEvidenceMilliseconds: supplementalLatencyPercentile(hydrate, 0.95),
			RerankMilliseconds:          supplementalLatencyPercentile(rerank, 0.95),
			PipelineTotalMilliseconds:   supplementalLatencyPercentile(pipeline, 0.95),
		},
	}
}

func supplementalLatencyPercentile(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	return sorted[max(index, 0)]
}

func supplementalLatencyIntegrityFailures(
	cold SupplementalNoAnswerLatencyPhase,
	warm SupplementalNoAnswerLatencyPhase,
) []string {
	failures := make([]string, 0)
	for _, phase := range []SupplementalNoAnswerLatencyPhase{cold, warm} {
		for _, failure := range phase.Summary.Failures {
			if failure != supplementalLatencyFailure {
				failures = append(failures, phase.Name+": "+failure)
			}
		}
	}
	sort.Strings(failures)
	return failures
}

func supplementalLatencyConclusion(
	integrityPassed bool,
	coldP95 int64,
	warmP95 int64,
	maximumP95 int64,
) string {
	if !integrityPassed {
		return "diagnostic_integrity_failed"
	}
	if coldP95 > maximumP95 && warmP95 <= maximumP95 && warmP95 < coldP95 {
		return "cold_start_effect_observed"
	}
	if warmP95 > maximumP95 {
		return "warm_state_latency_exceeded"
	}
	if warmP95 <= maximumP95 {
		return "warm_state_within_budget_cold_reproduction_inconclusive"
	}
	return "inconclusive"
}

func supplementalLatencyReductionRate(coldP95 int64, warmP95 int64) float64 {
	if coldP95 <= 0 {
		return 0
	}
	return float64(coldP95-warmP95) / float64(coldP95)
}

func validateSupplementalLatencyDiagnostic(
	diagnostic SupplementalNoAnswerLatencyDiagnostic,
) error {
	if diagnostic.SchemaVersion != SupplementalNoAnswerLatencyDiagnosticVersion ||
		diagnostic.CaptureVersion != CaptureVersion ||
		diagnostic.PromotionEvidence ||
		strings.TrimSpace(diagnostic.Suite.ID) == "" ||
		!validCaptureHash(diagnostic.Suite.RawSHA256) ||
		diagnostic.Suite.Cases != supplementalNoAnswerCaseCount ||
		!validCaptureUUID(diagnostic.Candidate.GenerationID) ||
		!validCaptureHash(diagnostic.Candidate.ArtifactManifestHash) ||
		!validCaptureHash(diagnostic.Candidate.ChunkProfileHash) ||
		diagnostic.Candidate.Status != "verified" ||
		diagnostic.Candidate.Readiness != "ready" ||
		diagnostic.Configuration.RetrievalProvider !=
			captureProviderConfiguration(CaptureRetrievalProvider{
				Profile: ragproviders.SiliconFlowRetrievalProfile,
			}) ||
		diagnostic.Configuration.AnswerProviderID != "SERVER_DEFAULT" ||
		strings.TrimSpace(diagnostic.Configuration.AnswerModelID) == "" ||
		diagnostic.Configuration.Concurrency != supplementalLatencyPhaseCases ||
		diagnostic.Configuration.ScoringPolicy != captureScoringPolicy ||
		diagnostic.Configuration.GenerationHeadRevision < 1 ||
		diagnostic.Configuration.CorpusProjectionRevision < 1 ||
		!validSupplementalNoAnswerCriteria(diagnostic.Criteria) ||
		diagnostic.SelectionPolicy != supplementalLatencySelectionPolicy ||
		diagnostic.Cold.Name != "cold" || diagnostic.Warm.Name != "warm" ||
		len(diagnostic.Cold.Cases) != supplementalLatencyPhaseCases ||
		len(diagnostic.Warm.Cases) != supplementalLatencyPhaseCases ||
		len(diagnostic.Cold.CaseIDs) != supplementalLatencyPhaseCases ||
		len(diagnostic.Warm.CaseIDs) != supplementalLatencyPhaseCases ||
		!validCaptureHash(diagnostic.SourceReport.RawSHA256) {
		return errors.New("supplemental latency diagnostic header is invalid")
	}
	capturedAt, err := time.Parse(time.RFC3339, diagnostic.CapturedAt)
	if err != nil {
		return errors.New("supplemental latency diagnostic timestamp is invalid")
	}
	sourceCapturedAt, err := time.Parse(time.RFC3339, diagnostic.SourceReport.CapturedAt)
	if err != nil || sourceCapturedAt.After(capturedAt) ||
		diagnostic.SourceReport.P50LatencyMilliseconds < 0 ||
		diagnostic.SourceReport.P95LatencyMilliseconds <=
			diagnostic.Criteria.MaximumP95LatencyMilliseconds ||
		diagnostic.SourceReport.MaximumLatencyMS <
			diagnostic.SourceReport.P95LatencyMilliseconds {
		return errors.New("supplemental latency diagnostic source is invalid")
	}
	seenCaseIDs := make(map[string]struct{}, supplementalLatencyPhaseCases*2)
	for _, phase := range []SupplementalNoAnswerLatencyPhase{
		diagnostic.Cold,
		diagnostic.Warm,
	} {
		if err := validateSupplementalNoAnswerObservationSet(
			phase.Cases,
			false,
		); err != nil {
			return err
		}
		caseIDs := make([]string, len(phase.Cases))
		for index, item := range phase.Cases {
			caseIDs[index] = item.CaseID
			if _, duplicate := seenCaseIDs[item.CaseID]; duplicate {
				return errors.New("supplemental latency diagnostic repeats a case")
			}
			seenCaseIDs[item.CaseID] = struct{}{}
		}
		expectedSummary := summarizeSupplementalNoAnswer(
			phase.Cases,
			diagnostic.Criteria,
			true,
		)
		if !reflect.DeepEqual(phase.CaseIDs, caseIDs) ||
			!reflect.DeepEqual(phase.Summary, expectedSummary) ||
			!reflect.DeepEqual(phase.Metrics, supplementalLatencyMetrics(phase.Cases)) {
			return errors.New("supplemental latency diagnostic phase is invalid")
		}
	}
	failures := supplementalLatencyIntegrityFailures(diagnostic.Cold, diagnostic.Warm)
	conclusion := supplementalLatencyConclusion(
		len(failures) == 0,
		diagnostic.Cold.Metrics.P95LatencyMilliseconds,
		diagnostic.Warm.Metrics.P95LatencyMilliseconds,
		diagnostic.Criteria.MaximumP95LatencyMilliseconds,
	)
	expectedDelta := SupplementalNoAnswerLatencyDelta{
		P95ReductionMilliseconds: diagnostic.Cold.Metrics.P95LatencyMilliseconds -
			diagnostic.Warm.Metrics.P95LatencyMilliseconds,
		P95ReductionRate: supplementalLatencyReductionRate(
			diagnostic.Cold.Metrics.P95LatencyMilliseconds,
			diagnostic.Warm.Metrics.P95LatencyMilliseconds,
		),
	}
	if !reflect.DeepEqual(diagnostic.Failures, failures) ||
		diagnostic.DiagnosticIntegrityPassed != (len(failures) == 0) ||
		diagnostic.Conclusion != conclusion ||
		!reflect.DeepEqual(diagnostic.Delta, expectedDelta) {
		return errors.New("supplemental latency diagnostic conclusion is invalid")
	}
	return nil
}

func WriteSupplementalNoAnswerLatencyDiagnosticExclusive(
	path string,
	diagnostic SupplementalNoAnswerLatencyDiagnostic,
	pretty bool,
) error {
	if err := validateSupplementalLatencyDiagnostic(diagnostic); err != nil {
		return err
	}
	return writeJSONExclusive(
		path,
		diagnostic,
		pretty,
		"supplemental no-answer latency diagnostic",
	)
}
