package ragevalcapture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

type supplementalNoAnswerAnswerer struct{}

func (supplementalNoAnswerAnswerer) Answer(
	context.Context,
	string,
	string,
) (AnswerResult, error) {
	return AnswerResult{Content: "INSUFFICIENT_EVIDENCE"}, nil
}

func TestValidateSupplementalNoAnswerSuiteRequiresFrozenCoverage(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*SupplementalNoAnswerInput)
	}{
		{
			name: "exactly 50 cases",
			mutate: func(input *SupplementalNoAnswerInput) {
				input.LoadedSuite.Suite.Cases = input.LoadedSuite.Suite.Cases[:49]
			},
		},
		{
			name: "25 cases per language",
			mutate: func(input *SupplementalNoAnswerInput) {
				input.LoadedSuite.Suite.Cases[0].Language = "english"
			},
		},
		{
			name: "10 cases per format",
			mutate: func(input *SupplementalNoAnswerInput) {
				item := &input.LoadedSuite.Suite.Cases[0]
				item.Format = "docx"
				item.AbsentSourceName = strings.TrimSuffix(
					item.AbsentSourceName,
					".pdf",
				) + ".docx"
				item.Query = strings.ReplaceAll(
					item.Query,
					".pdf",
					".docx",
				)
			},
		},
		{
			name: "absent filename does not exist in imported corpus",
			mutate: func(input *SupplementalNoAnswerInput) {
				input.ImportReceipt.Documents = []SourceImportDocument{{
					Filename: input.LoadedSuite.Suite.Cases[0].AbsentSourceName,
				}}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newSupplementalNoAnswerFixture(t)
			test.mutate(&input)
			if err := validateSupplementalNoAnswerSuite(input); err == nil {
				t.Fatal("invalid supplemental suite was accepted")
			}
		})
	}
}

func TestValidateSupplementalNoAnswerBindingRejectsDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*SupplementalNoAnswerInput, *GenerationStatus)
	}{
		{
			name: "candidate generation",
			mutate: func(input *SupplementalNoAnswerInput, _ *GenerationStatus) {
				input.LoadedSuite.Suite.Binding.CandidateGenerationID =
					"99999999-9999-4999-8999-999999999999"
			},
		},
		{
			name: "generation head revision",
			mutate: func(_ *SupplementalNoAnswerInput, status *GenerationStatus) {
				status.HeadRevision++
			},
		},
		{
			name: "corpus projection revision",
			mutate: func(input *SupplementalNoAnswerInput, _ *GenerationStatus) {
				input.LoadedSuite.Suite.Binding.CorpusProjectionRevision++
			},
		},
		{
			name: "retrieval profile",
			mutate: func(input *SupplementalNoAnswerInput, _ *GenerationStatus) {
				input.LoadedSuite.Suite.Binding.RetrievalProfileID = "jina_v4_v3"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newSupplementalNoAnswerFixture(t)
			status, err := input.Store.Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&input, &status)
			if err := validateSupplementalNoAnswerBinding(input, status); err == nil {
				t.Fatal("drifted supplemental binding was accepted")
			}
		})
	}
}

func TestCaptureSupplementalNoAnswerIsStableAndNonPromotional(t *testing.T) {
	input := newSupplementalNoAnswerFixture(t)
	report, err := CaptureSupplementalNoAnswer(context.Background(), input)
	if err != nil {
		t.Fatalf("CaptureSupplementalNoAnswer() error = %v", err)
	}
	if !report.Passed || report.PromotionEvidence || len(report.Cases) != 50 {
		t.Fatalf("supplemental report header = %#v", report)
	}
	for index, observation := range report.Cases {
		if observation.CaseID != input.LoadedSuite.Suite.Cases[index].ID {
			t.Fatalf("case %d = %q", index, observation.CaseID)
		}
		if observation.Answered || observation.CitationEvidenceCount != 0 ||
			observation.CitationMarkerCount != 0 {
			t.Fatalf("case %d observation = %#v", index, observation)
		}
	}
	if report.Summary.FalseAnswers != 0 || len(report.Slices) != 7 ||
		report.Slices["chinese"].Cases != 25 ||
		report.Slices["english"].Cases != 25 {
		t.Fatalf("supplemental summary/slices = %#v/%#v", report.Summary, report.Slices)
	}
}

func TestSummarizeSupplementalNoAnswerRejectsFalseAnswerCitationAndLeakage(
	t *testing.T,
) {
	criteria := supplementalNoAnswerCriteriaFixture()
	summary := summarizeSupplementalNoAnswer(
		[]SupplementalNoAnswerObservation{{
			CaseID:                "case-1",
			Answered:              true,
			CitationEvidenceCount: 1,
			CitationMarkerCount:   1,
			AbsentSourceMatched:   true,
			AbsentSubjectMatched:  true,
			Leakage: rageval.PromotionCaseLeakage{
				ACL:                  true,
				Deletion:             true,
				Secret:               true,
				UnauthorizedEvidence: true,
			},
		}},
		criteria,
		true,
	)
	if summary.Passed || summary.FalseAnswers != 1 ||
		summary.CasesWithCitationEvidence != 1 ||
		summary.CasesWithCitationMarkers != 1 ||
		summary.AbsentSourceMatches != 1 || summary.AbsentSubjectMatches != 1 ||
		summary.ACLLeaks != 1 || summary.DeletionLeaks != 1 ||
		summary.SecretLeaks != 1 || summary.UnauthorizedEvidenceLeaks != 1 ||
		len(summary.Failures) < 4 {
		t.Fatalf("failing supplemental summary = %#v", summary)
	}
}

func TestWriteSupplementalNoAnswerReportExclusiveNeverOverwrites(t *testing.T) {
	input := newSupplementalNoAnswerFixture(t)
	report, err := CaptureSupplementalNoAnswer(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "supplemental.json")
	if err := WriteSupplementalNoAnswerReportExclusive(path, report, true); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSupplementalNoAnswerReportExclusive(path, report, true); err == nil {
		t.Fatal("existing supplemental report was overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("existing supplemental report changed")
	}
}

func newSupplementalNoAnswerFixture(t *testing.T) SupplementalNoAnswerInput {
	t.Helper()
	base := newFrozenHoldoutFixture(t).CaptureInput
	base.Answerer = supplementalNoAnswerAnswerer{}
	base.Splits = nil
	base.CaseID = ""
	base.MaximumCases = 0
	status, err := base.Store.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	suite := SupplementalNoAnswerSuite{
		SchemaVersion:     SupplementalNoAnswerSuiteVersion,
		ID:                "candidate-8-supplemental-no-answer",
		Description:       "Synthetic no-answer regression; not promotion evidence.",
		Synthetic:         true,
		PromotionEvidence: false,
		CreatedAt:         base.Clock().Add(-time.Minute).UTC().Format(time.RFC3339),
		Binding: SupplementalNoAnswerBinding{
			GoldenSetID:              base.Golden.ID,
			GoldenRawSHA256:          base.GoldenRawSHA256,
			GoldenContentSHA256:      base.Golden.Lifecycle.FrozenContentSHA256,
			CurationRawSHA256:        base.CurationRawSHA256,
			HumanReviewRawSHA256:     base.ReviewRawSHA256,
			SourceImportRawSHA256:    base.ImportRawSHA256,
			CollectionID:             base.Curation.CollectionBinding.CollectionID,
			CandidateGenerationID:    status.CandidateGenerationID,
			ArtifactManifestHash:     status.CandidateArtifactManifestHash,
			ChunkProfileHash:         status.CandidateChunkProfileHash,
			RetrievalProfileID:       string(ragproviders.RetrievalProfileSiliconFlow),
			AnswerModelID:            base.AnswerModelID,
			GenerationHeadRevision:   status.HeadRevision,
			CorpusProjectionRevision: status.CorpusProjectionRevision,
		},
		Criteria: supplementalNoAnswerCriteriaFixture(),
		Cases:    supplementalNoAnswerCasesFixture(),
	}
	return SupplementalNoAnswerInput{
		CaptureInput: base,
		LoadedSuite: LoadedSupplementalNoAnswerSuite{
			Suite: suite, RawSHA256: strings.Repeat("9", 64),
		},
	}
}

func supplementalNoAnswerCriteriaFixture() SupplementalNoAnswerCriteria {
	return SupplementalNoAnswerCriteria{
		MaximumFalseAnswerRate:        0.02,
		MaximumP95LatencyMilliseconds: 1000,
		MaximumAverageContextTokens:   4096,
		RequireZeroCitationEvidence:   true,
		RequireZeroCitationMarkers:    true,
		RequireZeroAuthorityLeakage:   true,
		RequireAbsentSourceAndSubject: true,
	}
}

func supplementalNoAnswerCasesFixture() []SupplementalNoAnswerCase {
	result := make([]SupplementalNoAnswerCase, 0, 50)
	for _, format := range supplementalNoAnswerFormats {
		extension := format
		if format == "json_code" {
			extension = "md"
		}
		for index := 1; index <= 10; index++ {
			language := "chinese"
			if index > 5 {
				language = "english"
			}
			ordinal := fmtCaseID(index)[5:]
			name := "absent-" + format + "-" + language + "-" + ordinal + "." + extension
			token := "QZ-NOANSWER-" + strings.ToUpper(format) + "-" +
				strings.ToUpper(language) + "-" + ordinal
			result = append(result, SupplementalNoAnswerCase{
				ID:                 "no-answer-" + format + "-" + language + "-" + ordinal,
				Query:              "Find " + token + " in " + name + ".",
				Language:           language,
				Format:             format,
				ExpectedNoAnswer:   true,
				AbsentSourceName:   name,
				AbsentSubjectToken: token,
			})
		}
	}
	return result
}
