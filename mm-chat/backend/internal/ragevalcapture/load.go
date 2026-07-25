package ragevalcapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
)

const maximumCaptureInputBytes = 64 << 20

var (
	captureUUIDPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	captureSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	captureAnchorPattern = regexp.MustCompile(`^F[0-9]{2}$`)
)

func LoadInputs(
	goldenPath string,
	curationPath string,
	reviewPath string,
	importPath string,
) (LoadedInputs, error) {
	goldenBody, goldenHash, err := readCaptureFile(goldenPath)
	if err != nil {
		return LoadedInputs{}, fmt.Errorf("read promotion Golden: %w", err)
	}
	golden, err := rageval.DecodePromotionGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		return LoadedInputs{}, err
	}
	if err := rageval.ValidatePromotionGoldenAdmission(golden); err != nil {
		return LoadedInputs{}, err
	}

	curationBody, curationHash, err := readCaptureFile(curationPath)
	if err != nil {
		return LoadedInputs{}, fmt.Errorf("read curation queue: %w", err)
	}
	var curation CurationQueue
	if err := decodeCaptureJSON(curationBody, &curation); err != nil {
		return LoadedInputs{}, fmt.Errorf("decode curation queue: %w", err)
	}

	reviewBody, reviewHash, err := readCaptureFile(reviewPath)
	if err != nil {
		return LoadedInputs{}, fmt.Errorf("read human-review receipt: %w", err)
	}
	var review HumanReviewReceipt
	if err := decodeCaptureJSON(reviewBody, &review); err != nil {
		return LoadedInputs{}, fmt.Errorf("decode human-review receipt: %w", err)
	}

	importBody, importHash, err := readCaptureFile(importPath)
	if err != nil {
		return LoadedInputs{}, fmt.Errorf("read source import receipt: %w", err)
	}
	var sourceImport SourceImportReceipt
	if err := decodeCaptureJSON(importBody, &sourceImport); err != nil {
		return LoadedInputs{}, fmt.Errorf("decode source import receipt: %w", err)
	}

	loaded := LoadedInputs{
		Golden:             golden,
		GoldenRawSHA256:    goldenHash,
		Curation:           curation,
		CurationRawSHA256:  curationHash,
		ReviewReceipt:      review,
		ReviewRawSHA256:    reviewHash,
		ImportReceipt:      sourceImport,
		ImportRawSHA256:    importHash,
		CuratedByCaseID:    make(map[string]CurationCase, len(curation.Cases)),
		SourceByDocumentID: make(map[string]string, len(sourceImport.Documents)),
	}
	if err := validateLoadedInputs(&loaded); err != nil {
		return LoadedInputs{}, err
	}
	return loaded, nil
}

func LoadPreflightReport(path string) (LoadedPreflightReport, error) {
	body, digest, err := readCaptureFile(path)
	if err != nil {
		return LoadedPreflightReport{}, fmt.Errorf("read preflight report: %w", err)
	}
	var report PreflightReport
	if err := decodeCaptureJSON(body, &report); err != nil {
		return LoadedPreflightReport{}, fmt.Errorf("decode preflight report: %w", err)
	}
	return LoadedPreflightReport{Report: report, RawSHA256: digest}, nil
}

func readCaptureFile(path string) ([]byte, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", errors.New("path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumCaptureInputBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) == 0 || len(body) > maximumCaptureInputBytes {
		return nil, "", errors.New("capture input size is invalid")
	}
	digest := sha256.Sum256(body)
	return body, hex.EncodeToString(digest[:]), nil
}

func decodeCaptureJSON(body []byte, target any) error {
	duplicateDecoder := json.NewDecoder(bytes.NewReader(body))
	if err := rejectCaptureDuplicateKeys(duplicateDecoder); err != nil {
		return err
	}
	if token, err := duplicateDecoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON token %v", token)
		}
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func rejectCaptureDuplicateKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := rejectCaptureDuplicateKeys(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := rejectCaptureDuplicateKeys(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	expected := json.Delim('}')
	if delimiter == '[' {
		expected = ']'
	}
	if closing != expected {
		return errors.New("JSON delimiter is not balanced")
	}
	return nil
}

func validateLoadedInputs(loaded *LoadedInputs) error {
	golden := loaded.Golden
	curation := loaded.Curation
	review := loaded.ReviewReceipt
	sourceImport := loaded.ImportReceipt
	if curation.SchemaVersion != "neo-chat.rag-promotion-curation-queue.v1" ||
		!curation.Synthetic || curation.ReviewState != "draft" ||
		curation.PromotionEligible || len(curation.Cases) != len(golden.Cases) ||
		!validCaptureHash(curation.ImportReceiptSHA256) ||
		!validCaptureHash(curation.SourceManifest.SHA256) ||
		!validCaptureUUID(curation.CollectionBinding.CollectionID) ||
		strings.TrimSpace(curation.CollectionBinding.Alias) == "" {
		return errors.New("curation queue header is invalid")
	}
	if review.SchemaVersion != "neo-chat.rag-promotion-human-review-receipt.v1" ||
		review.Decision != "all_cases_human_reviewed" ||
		review.Attestation != "case_by_case_review_confirmed" ||
		review.CaseCount != len(golden.Cases) ||
		!validCaptureUUID(review.ReviewerID) ||
		review.CaseReviewState != "human_reviewed" ||
		review.PromotionGateEvaluated || review.PromotionEligible ||
		review.ActivationExecuted ||
		review.CurationQueueSHA256 != loaded.CurationRawSHA256 ||
		review.FrozenGoldenRawSHA256 != loaded.GoldenRawSHA256 ||
		review.FrozenContentSHA256 != golden.Lifecycle.FrozenContentSHA256 ||
		review.HoldoutRunID != golden.Lifecycle.HoldoutRunID ||
		review.FrozenAt != golden.Lifecycle.FrozenAt {
		return errors.New("human-review receipt binding is invalid")
	}
	reviewedAt, err := time.Parse(time.RFC3339, review.ReviewedAt)
	if err != nil {
		return errors.New("human-review receipt timestamp is invalid")
	}
	frozenAt, _ := time.Parse(time.RFC3339, golden.Lifecycle.FrozenAt)
	if reviewedAt.After(frozenAt) {
		return errors.New("human review occurred after corpus freeze")
	}
	if sourceImport.SchemaVersion != "neo-chat.rag-evaluation-source-import.v1" ||
		sourceImport.CollectionID != curation.CollectionBinding.CollectionID ||
		sourceImport.ManifestSHA256 != curation.SourceManifest.SHA256 ||
		loaded.ImportRawSHA256 != curation.ImportReceiptSHA256 ||
		len(sourceImport.Documents) == 0 {
		return errors.New("source import receipt binding is invalid")
	}

	importBySource := make(map[string]SourceImportDocument, len(sourceImport.Documents))
	for _, document := range sourceImport.Documents {
		if strings.TrimSpace(document.SourceID) == "" ||
			!validCaptureUUID(document.DocumentID) ||
			!validCaptureUUID(document.FileID) ||
			!validCaptureHash(document.SHA256) ||
			document.FinalStatus != "active" || document.VersionStatus != "active" {
			return fmt.Errorf("source import document %q is invalid", document.SourceID)
		}
		if _, duplicate := importBySource[document.SourceID]; duplicate {
			return fmt.Errorf("duplicate imported source %q", document.SourceID)
		}
		importBySource[document.SourceID] = document
		loaded.SourceByDocumentID[document.DocumentID] = document.SourceID
	}

	goldenByID := make(map[string]rageval.PromotionGoldenCase, len(golden.Cases))
	for _, item := range golden.Cases {
		goldenByID[item.ID] = item
	}
	for _, curated := range curation.Cases {
		caseID := curated.PromotionCase.ID
		frozen, ok := goldenByID[caseID]
		if !ok || curated.PromotionCase.Review.State != "draft" ||
			strings.TrimSpace(curated.ExpectedLabel) == "" ||
			strings.TrimSpace(curated.ExpectedAnswer) == "" ||
			strings.TrimSpace(curated.SourceBinding.SourceID) == "" ||
			!captureAnchorPattern.MatchString(curated.SourceBinding.Anchor) ||
			len(curated.PromotionCase.ExpectedRelevantEvidenceIDs) != 1 ||
			curated.PromotionCase.ExpectedRelevantEvidenceIDs[0] !=
				curated.SourceBinding.SourceID+":"+curated.SourceBinding.Anchor {
			return fmt.Errorf("curation case %q is invalid", caseID)
		}
		draftComparable := curated.PromotionCase
		draftComparable.Review = rageval.PromotionReview{}
		frozenComparable := frozen
		frozenComparable.Review = rageval.PromotionReview{}
		if !reflect.DeepEqual(draftComparable, frozenComparable) {
			return fmt.Errorf("curation case %q differs from frozen Golden", caseID)
		}
		imported, ok := importBySource[curated.SourceBinding.SourceID]
		if !ok || imported.DocumentID != curated.SourceBinding.DocumentID ||
			imported.FileID != curated.SourceBinding.FileID ||
			imported.Filename != curated.SourceBinding.Filename ||
			imported.SHA256 != curated.SourceBinding.SourceSHA256 {
			return fmt.Errorf("curation case %q source binding is invalid", caseID)
		}
		if _, duplicate := loaded.CuratedByCaseID[caseID]; duplicate {
			return fmt.Errorf("duplicate curation case %q", caseID)
		}
		loaded.CuratedByCaseID[caseID] = curated
	}
	if len(loaded.CuratedByCaseID) != len(golden.Cases) {
		return errors.New("curation cases do not exactly match frozen Golden")
	}
	return nil
}

func validCaptureUUID(value string) bool {
	return captureUUIDPattern.MatchString(strings.TrimSpace(value)) &&
		value != "00000000-0000-0000-0000-000000000000"
}

func validCaptureHash(value string) bool {
	return captureSHA256Pattern.MatchString(strings.TrimSpace(value))
}
