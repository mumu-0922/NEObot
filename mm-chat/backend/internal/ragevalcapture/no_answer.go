package ragevalcapture

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	SupplementalNoAnswerSuiteVersion  = "neo-chat.rag-supplemental-no-answer.v1"
	SupplementalNoAnswerReportVersion = "neo-chat.rag-supplemental-no-answer-report.v1"
	supplementalNoAnswerCaseCount     = 50
)

var supplementalNoAnswerFormats = [...]string{
	"pdf",
	"docx",
	"pptx",
	"xlsx",
	"json_code",
}

type SupplementalNoAnswerSuite struct {
	SchemaVersion     string                       `json:"schemaVersion"`
	ID                string                       `json:"id"`
	Description       string                       `json:"description"`
	Synthetic         bool                         `json:"synthetic"`
	PromotionEvidence bool                         `json:"promotionEvidence"`
	CreatedAt         string                       `json:"createdAt"`
	Binding           SupplementalNoAnswerBinding  `json:"binding"`
	Criteria          SupplementalNoAnswerCriteria `json:"criteria"`
	Cases             []SupplementalNoAnswerCase   `json:"cases"`
}

type SupplementalNoAnswerBinding struct {
	GoldenSetID              string `json:"goldenSetId"`
	GoldenRawSHA256          string `json:"goldenRawSha256"`
	GoldenContentSHA256      string `json:"goldenContentSha256"`
	CurationRawSHA256        string `json:"curationRawSha256"`
	HumanReviewRawSHA256     string `json:"humanReviewRawSha256"`
	SourceImportRawSHA256    string `json:"sourceImportRawSha256"`
	CollectionID             string `json:"collectionId"`
	CandidateGenerationID    string `json:"candidateGenerationId"`
	ArtifactManifestHash     string `json:"artifactManifestHash"`
	ChunkProfileHash         string `json:"chunkProfileHash"`
	RetrievalProfileID       string `json:"retrievalProfileId"`
	AnswerModelID            string `json:"answerModelId"`
	GenerationHeadRevision   int64  `json:"generationHeadRevision"`
	CorpusProjectionRevision int64  `json:"corpusProjectionRevision"`
}

type SupplementalNoAnswerCriteria struct {
	MaximumFalseAnswerRate        float64 `json:"maximumFalseAnswerRate"`
	MaximumP95LatencyMilliseconds int64   `json:"maximumP95LatencyMilliseconds"`
	MaximumAverageContextTokens   float64 `json:"maximumAverageContextTokens"`
	RequireZeroCitationEvidence   bool    `json:"requireZeroCitationEvidence"`
	RequireZeroCitationMarkers    bool    `json:"requireZeroCitationMarkers"`
	RequireZeroAuthorityLeakage   bool    `json:"requireZeroAuthorityLeakage"`
	RequireAbsentSourceAndSubject bool    `json:"requireAbsentSourceAndSubject"`
}

type SupplementalNoAnswerCase struct {
	ID                 string `json:"id"`
	Query              string `json:"query"`
	Language           string `json:"language"`
	Format             string `json:"format"`
	ExpectedNoAnswer   bool   `json:"expectedNoAnswer"`
	AbsentSourceName   string `json:"absentSourceName"`
	AbsentSubjectToken string `json:"absentSubjectToken"`
}

type LoadedSupplementalNoAnswerSuite struct {
	Suite     SupplementalNoAnswerSuite
	RawSHA256 string
}

type SupplementalNoAnswerInput struct {
	CaptureInput
	LoadedSuite LoadedSupplementalNoAnswerSuite
}

type SupplementalNoAnswerReport struct {
	SchemaVersion     string                                 `json:"schemaVersion"`
	CaptureVersion    string                                 `json:"captureVersion"`
	PromotionEvidence bool                                   `json:"promotionEvidence"`
	Passed            bool                                   `json:"passed"`
	CapturedAt        string                                 `json:"capturedAt"`
	Suite             SupplementalNoAnswerSuiteSummary       `json:"suite"`
	Candidate         SupplementalNoAnswerCandidate          `json:"candidate"`
	Configuration     SupplementalNoAnswerConfiguration      `json:"configuration"`
	Criteria          SupplementalNoAnswerCriteria           `json:"criteria"`
	Summary           SupplementalNoAnswerSummary            `json:"summary"`
	Slices            map[string]SupplementalNoAnswerSummary `json:"slices"`
	Cases             []SupplementalNoAnswerObservation      `json:"cases"`
	Failures          []string                               `json:"failures"`
}

type SupplementalNoAnswerSuiteSummary struct {
	ID        string `json:"id"`
	RawSHA256 string `json:"rawSha256"`
	Cases     int    `json:"cases"`
}

type SupplementalNoAnswerCandidate struct {
	GenerationID         string `json:"generationId"`
	ArtifactManifestHash string `json:"artifactManifestHash"`
	ChunkProfileHash     string `json:"chunkProfileHash"`
	Status               string `json:"status"`
	Readiness            string `json:"readiness"`
}

type SupplementalNoAnswerConfiguration struct {
	RetrievalProvider        PreflightRetrievalProviderConfiguration `json:"retrievalProvider"`
	AnswerProviderID         string                                  `json:"answerProviderId"`
	AnswerModelID            string                                  `json:"answerModelId"`
	Concurrency              int                                     `json:"concurrency"`
	ScoringPolicy            string                                  `json:"scoringPolicy"`
	GenerationHeadRevision   int64                                   `json:"generationHeadRevision"`
	CorpusProjectionRevision int64                                   `json:"corpusProjectionRevision"`
}

type SupplementalNoAnswerSummary struct {
	Cases                     int      `json:"cases"`
	FalseAnswers              int      `json:"falseAnswers"`
	FalseAnswerRate           float64  `json:"falseAnswerRate"`
	CasesWithCitationEvidence int      `json:"casesWithCitationEvidence"`
	CasesWithCitationMarkers  int      `json:"casesWithCitationMarkers"`
	AbsentSourceMatches       int      `json:"absentSourceMatches"`
	AbsentSubjectMatches      int      `json:"absentSubjectMatches"`
	ACLLeaks                  int      `json:"aclLeaks"`
	DeletionLeaks             int      `json:"deletionLeaks"`
	SecretLeaks               int      `json:"secretLeaks"`
	UnauthorizedEvidenceLeaks int      `json:"unauthorizedEvidenceLeaks"`
	P95LatencyMilliseconds    int64    `json:"p95LatencyMilliseconds"`
	AverageContextTokens      float64  `json:"averageContextTokens"`
	Passed                    bool     `json:"passed"`
	Failures                  []string `json:"failures"`
}

type SupplementalNoAnswerObservation struct {
	CaseID                 string                       `json:"caseId"`
	Language               string                       `json:"language"`
	Format                 string                       `json:"format"`
	Answered               bool                         `json:"answered"`
	AnswerSHA256           string                       `json:"answerSha256"`
	RetrievedEvidenceCount int                          `json:"retrievedEvidenceCount"`
	FinalEvidenceCount     int                          `json:"finalEvidenceCount"`
	CitationEvidenceCount  int                          `json:"citationEvidenceCount"`
	CitationMarkerCount    int                          `json:"citationMarkerCount"`
	AbsentSourceMatched    bool                         `json:"absentSourceMatched"`
	AbsentSubjectMatched   bool                         `json:"absentSubjectMatched"`
	LatencyMilliseconds    int64                        `json:"latencyMilliseconds"`
	LatencyBreakdown       PreflightLatencyBreakdown    `json:"latencyBreakdown"`
	ContextTokens          int                          `json:"contextTokens"`
	Leakage                rageval.PromotionCaseLeakage `json:"leakage"`
}

func LoadSupplementalNoAnswerSuite(
	path string,
) (LoadedSupplementalNoAnswerSuite, error) {
	body, digest, err := readCaptureFile(path)
	if err != nil {
		return LoadedSupplementalNoAnswerSuite{}, fmt.Errorf(
			"read supplemental no-answer suite: %w",
			err,
		)
	}
	var suite SupplementalNoAnswerSuite
	if err := decodeCaptureJSON(body, &suite); err != nil {
		return LoadedSupplementalNoAnswerSuite{}, fmt.Errorf(
			"decode supplemental no-answer suite: %w",
			err,
		)
	}
	return LoadedSupplementalNoAnswerSuite{Suite: suite, RawSHA256: digest}, nil
}

func CaptureSupplementalNoAnswer(
	ctx context.Context,
	input SupplementalNoAnswerInput,
) (SupplementalNoAnswerReport, error) {
	if err := validateSupplementalNoAnswerInput(input); err != nil {
		return SupplementalNoAnswerReport{}, err
	}
	status, err := input.Store.Status(ctx)
	if err != nil {
		return SupplementalNoAnswerReport{}, err
	}
	if err := validateCaptureStatus(input.CaptureInput, status); err != nil {
		return SupplementalNoAnswerReport{}, err
	}
	if err := validateSupplementalNoAnswerBinding(input, status); err != nil {
		return SupplementalNoAnswerReport{}, err
	}
	observations, err := captureSupplementalNoAnswerCases(ctx, input)
	if err != nil {
		return SupplementalNoAnswerReport{}, err
	}
	summary, slices, failures := supplementalNoAnswerReportSummaries(
		observations,
		input.LoadedSuite.Suite.Criteria,
	)
	return SupplementalNoAnswerReport{
		SchemaVersion:     SupplementalNoAnswerReportVersion,
		CaptureVersion:    CaptureVersion,
		PromotionEvidence: false,
		Passed:            len(failures) == 0,
		CapturedAt:        input.Clock().UTC().Format(time.RFC3339),
		Suite: SupplementalNoAnswerSuiteSummary{
			ID: input.LoadedSuite.Suite.ID, RawSHA256: input.LoadedSuite.RawSHA256,
			Cases: len(input.LoadedSuite.Suite.Cases),
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
		Criteria: input.LoadedSuite.Suite.Criteria,
		Summary:  summary, Slices: slices, Cases: observations, Failures: failures,
	}, nil
}

func supplementalNoAnswerReportSummaries(
	observations []SupplementalNoAnswerObservation,
	criteria SupplementalNoAnswerCriteria,
) (
	SupplementalNoAnswerSummary,
	map[string]SupplementalNoAnswerSummary,
	[]string,
) {
	summary := summarizeSupplementalNoAnswer(
		observations,
		criteria,
		false,
	)
	slices := make(map[string]SupplementalNoAnswerSummary, 7)
	for _, language := range []string{"chinese", "english"} {
		slices[language] = summarizeSupplementalNoAnswer(
			filterSupplementalNoAnswer(observations, language, ""),
			criteria,
			true,
		)
	}
	for _, format := range supplementalNoAnswerFormats {
		slices[format] = summarizeSupplementalNoAnswer(
			filterSupplementalNoAnswer(observations, "", format),
			criteria,
			true,
		)
	}
	failures := append([]string(nil), summary.Failures...)
	for name, slice := range slices {
		for _, failure := range slice.Failures {
			failures = append(failures, name+": "+failure)
		}
	}
	sort.Strings(failures)
	return summary, slices, failures
}

func validateSupplementalNoAnswerInput(input SupplementalNoAnswerInput) error {
	base := input.CaptureInput
	if base.Store == nil ||
		!validCaptureProvider(base.CandidateProvider, ragproviders.SiliconFlowRetrievalProfile) ||
		base.Answerer == nil || base.Clock == nil ||
		!validCaptureUUID(base.CandidateGenerationID) ||
		!validCaptureHash(base.CandidateManifestHash) ||
		strings.TrimSpace(base.AnswerProviderID) == "" ||
		strings.TrimSpace(base.AnswerModelID) == "" ||
		base.Concurrency < 1 || base.Concurrency > 16 ||
		len(base.SourceByDocumentID) == 0 ||
		len(base.Splits) != 0 || strings.TrimSpace(base.CaseID) != "" ||
		base.MaximumCases != 0 || !validCaptureHash(input.LoadedSuite.RawSHA256) {
		return errors.New("supplemental no-answer input is invalid")
	}
	return validateSupplementalNoAnswerSuite(input)
}

func validateSupplementalNoAnswerSuite(input SupplementalNoAnswerInput) error {
	suite := input.LoadedSuite.Suite
	if suite.SchemaVersion != SupplementalNoAnswerSuiteVersion ||
		strings.TrimSpace(suite.ID) == "" || strings.TrimSpace(suite.Description) == "" ||
		!suite.Synthetic || suite.PromotionEvidence ||
		len(suite.Cases) != supplementalNoAnswerCaseCount {
		return errors.New("supplemental no-answer suite header is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339, suite.CreatedAt)
	if err != nil || createdAt.After(input.Clock().UTC()) {
		return errors.New("supplemental no-answer suite timestamp is invalid")
	}
	if !validSupplementalNoAnswerCriteria(suite.Criteria) {
		return errors.New("supplemental no-answer criteria are invalid")
	}
	importedNames := make(map[string]struct{}, len(input.ImportReceipt.Documents))
	for _, document := range input.ImportReceipt.Documents {
		importedNames[strings.ToLower(document.Filename)] = struct{}{}
	}
	caseIDs := make(map[string]struct{}, len(suite.Cases))
	sourceNames := make(map[string]struct{}, len(suite.Cases))
	subjects := make(map[string]struct{}, len(suite.Cases))
	languages := map[string]int{"chinese": 0, "english": 0}
	formats := make(map[string]int, len(supplementalNoAnswerFormats))
	for _, format := range supplementalNoAnswerFormats {
		formats[format] = 0
	}
	for _, item := range suite.Cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Query) == "" ||
			len([]byte(item.Query)) > 2048 || !item.ExpectedNoAnswer ||
			!validCaptureSourceName(item.AbsentSourceName) ||
			strings.TrimSpace(item.AbsentSubjectToken) == "" ||
			!strings.Contains(item.Query, item.AbsentSourceName) ||
			!strings.Contains(item.Query, item.AbsentSubjectToken) {
			return fmt.Errorf("supplemental no-answer case %q is invalid", item.ID)
		}
		if _, ok := languages[item.Language]; !ok {
			return fmt.Errorf("supplemental no-answer case %q language is invalid", item.ID)
		}
		if _, ok := formats[item.Format]; !ok ||
			!supplementalNoAnswerExtensionMatches(item.Format, item.AbsentSourceName) {
			return fmt.Errorf("supplemental no-answer case %q format is invalid", item.ID)
		}
		for value, seen := range map[string]map[string]struct{}{
			item.ID:                                  caseIDs,
			strings.ToLower(item.AbsentSourceName):   sourceNames,
			strings.ToUpper(item.AbsentSubjectToken): subjects,
		} {
			if _, duplicate := seen[value]; duplicate {
				return fmt.Errorf("supplemental no-answer case %q is duplicated", item.ID)
			}
			seen[value] = struct{}{}
		}
		if _, collision := importedNames[strings.ToLower(item.AbsentSourceName)]; collision {
			return fmt.Errorf("supplemental no-answer source %q exists", item.AbsentSourceName)
		}
		languages[item.Language]++
		formats[item.Format]++
	}
	if languages["chinese"] != 25 || languages["english"] != 25 {
		return errors.New("supplemental no-answer language coverage is invalid")
	}
	for format, count := range formats {
		if count != 10 {
			return fmt.Errorf("supplemental no-answer format %q coverage is invalid", format)
		}
	}
	return nil
}

func validateSupplementalNoAnswerBinding(
	input SupplementalNoAnswerInput,
	status GenerationStatus,
) error {
	binding := input.LoadedSuite.Suite.Binding
	if binding.GoldenSetID != input.Golden.ID ||
		binding.GoldenRawSHA256 != input.GoldenRawSHA256 ||
		binding.GoldenContentSHA256 != input.Golden.Lifecycle.FrozenContentSHA256 ||
		binding.CurationRawSHA256 != input.CurationRawSHA256 ||
		binding.HumanReviewRawSHA256 != input.ReviewRawSHA256 ||
		binding.SourceImportRawSHA256 != input.ImportRawSHA256 ||
		binding.CollectionID != input.Curation.CollectionBinding.CollectionID ||
		binding.CandidateGenerationID != status.CandidateGenerationID ||
		binding.ArtifactManifestHash != status.CandidateArtifactManifestHash ||
		binding.ChunkProfileHash != status.CandidateChunkProfileHash ||
		binding.RetrievalProfileID != string(ragproviders.RetrievalProfileSiliconFlow) ||
		binding.AnswerModelID != input.AnswerModelID ||
		binding.GenerationHeadRevision != status.HeadRevision ||
		binding.CorpusProjectionRevision != status.CorpusProjectionRevision {
		return errors.New("supplemental no-answer binding drifted")
	}
	return nil
}

func supplementalNoAnswerExtensionMatches(format string, name string) bool {
	extension := strings.ToLower(name)
	switch format {
	case "pdf":
		return strings.HasSuffix(extension, ".pdf")
	case "docx":
		return strings.HasSuffix(extension, ".docx")
	case "pptx":
		return strings.HasSuffix(extension, ".pptx")
	case "xlsx":
		return strings.HasSuffix(extension, ".xlsx")
	case "json_code":
		return strings.HasSuffix(extension, ".md")
	default:
		return false
	}
}

func captureSupplementalNoAnswerCases(
	ctx context.Context,
	input SupplementalNoAnswerInput,
) ([]SupplementalNoAnswerObservation, error) {
	return captureSupplementalNoAnswerCaseSet(
		ctx,
		input,
		input.LoadedSuite.Suite.Cases,
	)
}

func captureSupplementalNoAnswerCaseSet(
	ctx context.Context,
	input SupplementalNoAnswerInput,
	cases []SupplementalNoAnswerCase,
) ([]SupplementalNoAnswerObservation, error) {
	if len(cases) == 0 {
		return nil, errors.New("supplemental no-answer case set is empty")
	}
	results := make([]SupplementalNoAnswerObservation, len(cases))
	workerCount := min(input.Concurrency, len(cases))
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	errCh := make(chan error, 1)
	var wait sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				observation, err := captureSupplementalNoAnswerCase(
					workCtx,
					input,
					cases[index],
				)
				if err != nil {
					select {
					case errCh <- fmt.Errorf("capture case %q: %w", cases[index].ID, err):
						cancel()
					default:
					}
					return
				}
				results[index] = observation
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range cases {
			select {
			case <-workCtx.Done():
				return
			case jobs <- index:
			}
		}
	}()
	wait.Wait()
	select {
	case captureErr := <-errCh:
		return nil, captureErr
	default:
	}
	for index, result := range results {
		if result.CaseID != cases[index].ID {
			return nil, errors.New("supplemental no-answer ordering is invalid")
		}
	}
	return results, nil
}

func captureSupplementalNoAnswerCase(
	ctx context.Context,
	input SupplementalNoAnswerInput,
	item SupplementalNoAnswerCase,
) (SupplementalNoAnswerObservation, error) {
	result, err := captureProfileCase(
		ctx,
		input.CaptureInput,
		rageval.PromotionGoldenCase{
			ID: item.ID, Query: item.Query, ExpectedNoAnswer: true,
			SelectedCollectionAliases: []string{input.Curation.CollectionBinding.Alias},
		},
		CurationCase{},
		input.CandidateGenerationID,
		[]string{input.Curation.CollectionBinding.CollectionID},
		input.CandidateProvider,
	)
	if err != nil {
		return SupplementalNoAnswerObservation{}, err
	}
	absentSourceMatched := false
	absentSubjectMatched := false
	for _, evidence := range result.evidence {
		absentSourceMatched = absentSourceMatched || strings.EqualFold(
			strings.TrimSpace(evidence.SourceName),
			strings.TrimSpace(item.AbsentSourceName),
		)
		absentSubjectMatched = absentSubjectMatched || strings.Contains(
			strings.ToUpper(evidence.SourceText),
			strings.ToUpper(item.AbsentSubjectToken),
		)
	}
	return SupplementalNoAnswerObservation{
		CaseID: item.ID, Language: item.Language, Format: item.Format,
		Answered:               result.observation.Answered,
		AnswerSHA256:           result.observation.AnswerSHA256,
		RetrievedEvidenceCount: len(result.observation.RetrievedEvidenceIDs),
		FinalEvidenceCount:     len(result.observation.FinalEvidenceIDs),
		CitationEvidenceCount:  len(result.observation.CitationEvidenceIDs),
		CitationMarkerCount:    len(citationMarkerRE.FindAllString(result.answer, -1)),
		AbsentSourceMatched:    absentSourceMatched,
		AbsentSubjectMatched:   absentSubjectMatched,
		LatencyMilliseconds:    result.observation.LatencyMilliseconds,
		LatencyBreakdown:       result.observation.LatencyBreakdown,
		ContextTokens:          result.observation.ContextTokens,
		Leakage:                result.observation.Leakage,
	}, nil
}

func filterSupplementalNoAnswer(
	observations []SupplementalNoAnswerObservation,
	language string,
	format string,
) []SupplementalNoAnswerObservation {
	result := make([]SupplementalNoAnswerObservation, 0, len(observations))
	for _, item := range observations {
		if (language == "" || item.Language == language) &&
			(format == "" || item.Format == format) {
			result = append(result, item)
		}
	}
	return result
}

func summarizeSupplementalNoAnswer(
	observations []SupplementalNoAnswerObservation,
	criteria SupplementalNoAnswerCriteria,
	requireZeroFalseAnswers bool,
) SupplementalNoAnswerSummary {
	summary := SupplementalNoAnswerSummary{Cases: len(observations)}
	promotionObservations := make([]rageval.PromotionCaseObservation, len(observations))
	goldenCases := make([]rageval.PromotionGoldenCase, len(observations))
	for index, item := range observations {
		if item.Answered {
			summary.FalseAnswers++
		}
		if item.CitationEvidenceCount > 0 {
			summary.CasesWithCitationEvidence++
		}
		if item.CitationMarkerCount > 0 {
			summary.CasesWithCitationMarkers++
		}
		if item.AbsentSourceMatched {
			summary.AbsentSourceMatches++
		}
		if item.AbsentSubjectMatched {
			summary.AbsentSubjectMatches++
		}
		if item.Leakage.ACL {
			summary.ACLLeaks++
		}
		if item.Leakage.Deletion {
			summary.DeletionLeaks++
		}
		if item.Leakage.Secret {
			summary.SecretLeaks++
		}
		if item.Leakage.UnauthorizedEvidence {
			summary.UnauthorizedEvidenceLeaks++
		}
		goldenCases[index] = rageval.PromotionGoldenCase{
			ID: item.CaseID, ExpectedNoAnswer: true,
		}
		promotionObservations[index] = rageval.PromotionCaseObservation{
			CaseID: item.CaseID, Answered: item.Answered,
			LatencyMilliseconds: item.LatencyMilliseconds,
			ContextTokens:       item.ContextTokens,
		}
	}
	if summary.Cases > 0 {
		summary.FalseAnswerRate = float64(summary.FalseAnswers) / float64(summary.Cases)
	}
	profile, err := rageval.SummarizePromotionProfile(goldenCases, promotionObservations)
	if err == nil {
		summary.P95LatencyMilliseconds = profile.P95LatencyMilliseconds
		summary.AverageContextTokens = profile.AverageContextTokens
	}
	failures := make([]string, 0)
	if summary.Cases == 0 {
		failures = append(failures, "no cases were evaluated")
	}
	if (requireZeroFalseAnswers && summary.FalseAnswers != 0) ||
		(!requireZeroFalseAnswers && summary.FalseAnswerRate > criteria.MaximumFalseAnswerRate) {
		failures = append(failures, "false-answer rate exceeded the bound")
	}
	if summary.CasesWithCitationEvidence != 0 {
		failures = append(failures, "no-answer response minted Citation evidence")
	}
	if summary.CasesWithCitationMarkers != 0 {
		failures = append(failures, "no-answer response emitted Citation markers")
	}
	if summary.AbsentSourceMatches != 0 || summary.AbsentSubjectMatches != 0 {
		failures = append(failures, "an absent source or subject appeared in answer context")
	}
	if summary.ACLLeaks != 0 || summary.DeletionLeaks != 0 ||
		summary.SecretLeaks != 0 || summary.UnauthorizedEvidenceLeaks != 0 {
		failures = append(failures, "authority or secret leakage was observed")
	}
	if summary.P95LatencyMilliseconds > criteria.MaximumP95LatencyMilliseconds {
		failures = append(failures, "P95 retrieval latency exceeded the budget")
	}
	if summary.AverageContextTokens > criteria.MaximumAverageContextTokens {
		failures = append(failures, "average context exceeded the budget")
	}
	sort.Strings(failures)
	summary.Failures = failures
	summary.Passed = len(failures) == 0
	return summary
}

func WriteSupplementalNoAnswerReportExclusive(
	path string,
	report SupplementalNoAnswerReport,
	pretty bool,
) error {
	if err := validateSupplementalNoAnswerReport(report); err != nil {
		return err
	}
	return writeJSONExclusive(path, report, pretty, "supplemental no-answer report")
}
