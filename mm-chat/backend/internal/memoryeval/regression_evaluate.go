package memoryeval

import (
	"errors"
	"fmt"
)

// EvaluateRegression scores a machine-reviewed regression corpus without
// granting any Golden, human-review, freeze, Holdout, or promotion authority.
func EvaluateRegression(input RegressionEvaluationInput) (RegressionReport, error) {
	if err := validateRegressionCorpus(input.Corpus); err != nil {
		return RegressionReport{}, err
	}
	if err := validateRegressionAudit(input.Audit); err != nil {
		return RegressionReport{}, err
	}
	if err := validateRegressionAdmission(input.Corpus, input.Audit); err != nil {
		return RegressionReport{}, err
	}
	if err := validateRegressionObservationSet(input.Observations); err != nil {
		return RegressionReport{}, err
	}
	if err := validateRegressionBindings(input); err != nil {
		return RegressionReport{}, err
	}
	scored, err := scoreEvaluation(
		input.Corpus.Cases,
		input.Observations.Cases,
		input.Corpus.Criteria,
		input.Observations.Profile,
		input.Observations.Costs,
	)
	if err != nil {
		return RegressionReport{}, err
	}
	splits := splitCounts(input.Corpus.Cases)
	return RegressionReport{
		SchemaVersion:     RegressionReportSchemaVersion,
		Passed:            len(scored.failures) == 0,
		PromotionEligible: false,
		CorpusClass:       RegressionCorpusClass,
		AdmissionMode:     RegressionAdmissionMode,
		Evaluation: RegressionEvaluationProvenance{
			EvaluatorVersion:      RegressionEvaluatorVersion,
			CorpusRawSHA256:       input.CorpusRawSHA256,
			CorpusContentSHA256:   input.Corpus.CorpusContentSHA256,
			AuditRawSHA256:        input.AuditRawSHA256,
			AuditContentSHA256:    input.Audit.ContentSHA256,
			ObservationsRawSHA256: input.ObservationsSHA256,
			CaptureID:             input.Observations.CaptureID,
			FixtureManifestSHA256: input.Corpus.FixtureManifestSHA256,
		},
		Corpus: RegressionCorpusSummary{
			CorpusID:         input.Corpus.ID,
			AuditVerdict:     input.Audit.Verdict,
			TotalCases:       len(input.Corpus.Cases),
			DevelopmentCount: splits["development"],
			ValidationCount:  splits["validation"],
			HoldoutCount:     splits["holdout"],
		},
		Profile:  scored.profile,
		Slices:   scored.slices,
		Failures: scored.failures,
	}, nil
}

func validateRegressionBindings(input RegressionEvaluationInput) error {
	if !validSHA256(input.CorpusRawSHA256) || !validSHA256(input.AuditRawSHA256) ||
		!validSHA256(input.ObservationsSHA256) {
		return errors.New("Memory regression evaluation raw hashes are invalid")
	}
	observations := input.Observations
	if observations.CorpusID != input.Corpus.ID ||
		observations.CorpusContentSHA256 != input.Corpus.CorpusContentSHA256 ||
		observations.AuditContentSHA256 != input.Audit.ContentSHA256 ||
		observations.FixtureManifestSHA256 != input.Corpus.FixtureManifestSHA256 {
		return errors.New("Memory regression observations are not bound to the admitted corpus")
	}
	auditedAt, _ := parseTimestamp(input.Audit.AuditedAt)
	capturedAt, _ := parseTimestamp(observations.CapturedAt)
	if capturedAt.Before(auditedAt) {
		return errors.New("Memory regression observations predate corpus admission")
	}
	if len(observations.Cases) != len(input.Corpus.Cases) {
		return errors.New("Memory regression observations do not exactly match the corpus")
	}
	for index, regressionCase := range input.Corpus.Cases {
		if observations.Cases[index].CaseID != regressionCase.ID {
			return fmt.Errorf("Memory regression observation order differs at case %q", regressionCase.ID)
		}
	}
	return nil
}
