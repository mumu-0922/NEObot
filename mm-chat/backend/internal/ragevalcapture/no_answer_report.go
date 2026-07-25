package ragevalcapture

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

type LoadedSupplementalNoAnswerReport struct {
	Report    SupplementalNoAnswerReport
	RawSHA256 string
}

func LoadSupplementalNoAnswerReport(
	path string,
) (LoadedSupplementalNoAnswerReport, error) {
	body, digest, err := readCaptureFile(path)
	if err != nil {
		return LoadedSupplementalNoAnswerReport{}, fmt.Errorf(
			"read supplemental no-answer report: %w",
			err,
		)
	}
	var report SupplementalNoAnswerReport
	if err := decodeCaptureJSON(body, &report); err != nil {
		return LoadedSupplementalNoAnswerReport{}, fmt.Errorf(
			"decode supplemental no-answer report: %w",
			err,
		)
	}
	if err := validateSupplementalNoAnswerReport(report); err != nil {
		return LoadedSupplementalNoAnswerReport{}, err
	}
	return LoadedSupplementalNoAnswerReport{Report: report, RawSHA256: digest}, nil
}

func validateSupplementalNoAnswerReport(
	report SupplementalNoAnswerReport,
) error {
	if report.SchemaVersion != SupplementalNoAnswerReportVersion ||
		report.CaptureVersion != CaptureVersion || report.PromotionEvidence ||
		strings.TrimSpace(report.Suite.ID) == "" ||
		!validCaptureHash(report.Suite.RawSHA256) ||
		report.Suite.Cases != supplementalNoAnswerCaseCount ||
		len(report.Cases) != supplementalNoAnswerCaseCount ||
		!validCaptureUUID(report.Candidate.GenerationID) ||
		!validCaptureHash(report.Candidate.ArtifactManifestHash) ||
		!validCaptureHash(report.Candidate.ChunkProfileHash) ||
		report.Candidate.Status != "verified" ||
		report.Candidate.Readiness != "ready" ||
		report.Configuration.RetrievalProvider !=
			captureProviderConfiguration(CaptureRetrievalProvider{
				Profile: ragproviders.SiliconFlowRetrievalProfile,
			}) ||
		report.Configuration.AnswerProviderID != "SERVER_DEFAULT" ||
		strings.TrimSpace(report.Configuration.AnswerModelID) == "" ||
		report.Configuration.Concurrency < 1 ||
		report.Configuration.Concurrency > 16 ||
		report.Configuration.ScoringPolicy != captureScoringPolicy ||
		report.Configuration.GenerationHeadRevision < 1 ||
		report.Configuration.CorpusProjectionRevision < 1 ||
		!validSupplementalNoAnswerCriteria(report.Criteria) {
		return errors.New("supplemental no-answer report header is invalid")
	}
	if _, err := time.Parse(time.RFC3339, report.CapturedAt); err != nil {
		return errors.New("supplemental no-answer report timestamp is invalid")
	}
	if err := validateSupplementalNoAnswerObservations(report.Cases); err != nil {
		return err
	}
	summary, slices, failures := supplementalNoAnswerReportSummaries(
		report.Cases,
		report.Criteria,
	)
	if !reflect.DeepEqual(report.Summary, summary) ||
		!reflect.DeepEqual(report.Slices, slices) ||
		!reflect.DeepEqual(report.Failures, failures) ||
		report.Passed != (len(failures) == 0) {
		return errors.New("supplemental no-answer report summary is invalid")
	}
	return nil
}

func validSupplementalNoAnswerCriteria(
	criteria SupplementalNoAnswerCriteria,
) bool {
	return criteria.MaximumFalseAnswerRate == 0.02 &&
		criteria.MaximumP95LatencyMilliseconds == 1000 &&
		criteria.MaximumAverageContextTokens == 4096 &&
		criteria.RequireZeroCitationEvidence &&
		criteria.RequireZeroCitationMarkers &&
		criteria.RequireZeroAuthorityLeakage &&
		criteria.RequireAbsentSourceAndSubject
}

func validateSupplementalNoAnswerObservations(
	observations []SupplementalNoAnswerObservation,
) error {
	return validateSupplementalNoAnswerObservationSet(observations, true)
}

func validateSupplementalNoAnswerObservationSet(
	observations []SupplementalNoAnswerObservation,
	requireFullCoverage bool,
) error {
	caseIDs := make(map[string]struct{}, len(observations))
	languages := map[string]int{"chinese": 0, "english": 0}
	formats := make(map[string]int, len(supplementalNoAnswerFormats))
	for _, format := range supplementalNoAnswerFormats {
		formats[format] = 0
	}
	for _, observation := range observations {
		if strings.TrimSpace(observation.CaseID) == "" ||
			!validCaptureHash(observation.AnswerSHA256) ||
			observation.RetrievedEvidenceCount < 0 ||
			observation.FinalEvidenceCount < 0 ||
			observation.CitationEvidenceCount < 0 ||
			observation.FinalEvidenceCount > observation.RetrievedEvidenceCount ||
			observation.CitationEvidenceCount > observation.FinalEvidenceCount ||
			observation.CitationMarkerCount < 0 ||
			observation.LatencyMilliseconds < 0 ||
			observation.ContextTokens < 0 ||
			observation.ContextTokens > captureMaximumContextTokens ||
			!validSupplementalLatencyBreakdown(
				observation.LatencyBreakdown,
				observation.LatencyMilliseconds,
			) {
			return fmt.Errorf(
				"supplemental no-answer observation %q is invalid",
				observation.CaseID,
			)
		}
		if _, duplicate := caseIDs[observation.CaseID]; duplicate {
			return fmt.Errorf(
				"supplemental no-answer observation %q is duplicated",
				observation.CaseID,
			)
		}
		caseIDs[observation.CaseID] = struct{}{}
		if _, ok := languages[observation.Language]; !ok {
			return fmt.Errorf(
				"supplemental no-answer observation %q language is invalid",
				observation.CaseID,
			)
		}
		if _, ok := formats[observation.Format]; !ok {
			return fmt.Errorf(
				"supplemental no-answer observation %q format is invalid",
				observation.CaseID,
			)
		}
		languages[observation.Language]++
		formats[observation.Format]++
	}
	if !requireFullCoverage {
		return nil
	}
	if languages["chinese"] != 25 || languages["english"] != 25 {
		return errors.New("supplemental no-answer report language coverage is invalid")
	}
	for format, count := range formats {
		if count != 10 {
			return fmt.Errorf(
				"supplemental no-answer report format %q coverage is invalid",
				format,
			)
		}
	}
	return nil
}

func validSupplementalLatencyBreakdown(
	breakdown PreflightLatencyBreakdown,
	total int64,
) bool {
	return breakdown.EmbedQueryMilliseconds >= 0 &&
		breakdown.FetchCandidatesMilliseconds >= 0 &&
		breakdown.HydrateEvidenceMilliseconds >= 0 &&
		breakdown.RerankMilliseconds >= 0 &&
		breakdown.PipelineTotalMilliseconds == total
}
