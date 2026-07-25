package rageval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

func DecodePromotionGoldenSet(reader io.Reader) (PromotionGoldenSet, error) {
	var value PromotionGoldenSet
	if err := decodePromotionOne(reader, &value); err != nil {
		return PromotionGoldenSet{}, fmt.Errorf("decode promotion Golden corpus: %w", err)
	}
	if err := validatePromotionGoldenSet(value); err != nil {
		return PromotionGoldenSet{}, err
	}
	return value, nil
}

func DecodePromotionObservationSet(
	reader io.Reader,
) (PromotionObservationSet, error) {
	var value PromotionObservationSet
	if err := decodePromotionOne(reader, &value); err != nil {
		return PromotionObservationSet{}, fmt.Errorf(
			"decode promotion observations: %w",
			err,
		)
	}
	if err := validatePromotionObservationSet(value); err != nil {
		return PromotionObservationSet{}, err
	}
	return value, nil
}

func decodePromotionOne(reader io.Reader, target any) error {
	if reader == nil {
		return errors.New("input is required")
	}
	const maximumPromotionInputBytes = 64 << 20
	body, err := io.ReadAll(io.LimitReader(reader, maximumPromotionInputBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maximumPromotionInputBytes {
		return errors.New("promotion input exceeds 64 MiB")
	}
	duplicateDecoder := json.NewDecoder(bytes.NewReader(body))
	if err := rejectDuplicateJSONKeys(duplicateDecoder); err != nil {
		return err
	}
	if token, err := duplicateDecoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON token %v", token)
		}
		return err
	}
	return decodeOne(bytes.NewReader(body), target)
}

func rejectDuplicateJSONKeys(decoder *json.Decoder) error {
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
			if err := rejectDuplicateJSONKeys(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := rejectDuplicateJSONKeys(decoder); err != nil {
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
	expectedClosing := json.Delim('}')
	if delimiter == '[' {
		expectedClosing = ']'
	}
	if closing != expectedClosing {
		return errors.New("JSON delimiter is not balanced")
	}
	return nil
}

func PromotionGoldenContentSHA256(value PromotionGoldenSet) (string, error) {
	value.Lifecycle.FrozenContentSHA256 = ""
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode promotion Golden corpus: %w", err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func validatePromotionGoldenSet(value PromotionGoldenSet) error {
	if value.SchemaVersion != PromotionGoldenSchemaVersion ||
		strings.TrimSpace(value.ID) == "" ||
		strings.TrimSpace(value.Description) == "" ||
		len(value.Cases) == 0 {
		return errors.New("promotion Golden corpus header is invalid")
	}
	if value.Lifecycle.State != "draft" && value.Lifecycle.State != "frozen" {
		return errors.New("promotion Golden corpus lifecycle is invalid")
	}
	if value.Lifecycle.FrozenAt != "" {
		if _, err := parsePromotionTimestamp(value.Lifecycle.FrozenAt); err != nil {
			return errors.New("promotion Golden corpus frozenAt is invalid")
		}
	}
	if value.Lifecycle.HoldoutRunID != "" &&
		!validUUID(value.Lifecycle.HoldoutRunID) {
		return errors.New("promotion Golden corpus Holdout run id is invalid")
	}
	if value.Lifecycle.FrozenContentSHA256 != "" &&
		!validSHA256(value.Lifecycle.FrozenContentSHA256) {
		return errors.New("promotion Golden corpus frozen hash is invalid")
	}
	if value.Criteria.MaximumP95LatencyMilliseconds <= 0 ||
		value.Criteria.MaximumAverageContextTokens <= 0 ||
		math.IsNaN(value.Criteria.MaximumAverageContextTokens) ||
		math.IsInf(value.Criteria.MaximumAverageContextTokens, 0) {
		return errors.New("promotion Golden corpus criteria are invalid")
	}

	seen := make(map[string]struct{}, len(value.Cases))
	for _, item := range value.Cases {
		if strings.TrimSpace(item.ID) == "" ||
			strings.TrimSpace(item.Query) == "" ||
			len(item.Slices) == 0 ||
			len(item.SelectedCollectionAliases) == 0 {
			return fmt.Errorf("promotion Golden case %q is invalid", item.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("duplicate promotion Golden case %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if item.Split != "development" &&
			item.Split != "validation" &&
			item.Split != "holdout" {
			return fmt.Errorf("promotion Golden case %q split is invalid", item.ID)
		}
		if item.ExpectedNoAnswer == (len(item.ExpectedRelevantEvidenceIDs) > 0) {
			return fmt.Errorf(
				"promotion Golden case %q evidence expectation is inconsistent",
				item.ID,
			)
		}
		if hasBlankOrDuplicate(item.Slices) ||
			hasBlankOrDuplicate(item.SelectedCollectionAliases) ||
			hasBlankOrDuplicate(item.ExpectedRelevantEvidenceIDs) {
			return fmt.Errorf(
				"promotion Golden case %q contains blank or duplicate identifiers",
				item.ID,
			)
		}
		if item.Review.State != "draft" && item.Review.State != "human_reviewed" {
			return fmt.Errorf("promotion Golden case %q review is invalid", item.ID)
		}
		if item.Review.State == "human_reviewed" {
			if !validUUID(item.Review.ReviewerID) {
				return fmt.Errorf(
					"promotion Golden case %q reviewer identity is invalid",
					item.ID,
				)
			}
			if _, err := parsePromotionTimestamp(item.Review.ReviewedAt); err != nil {
				return fmt.Errorf(
					"promotion Golden case %q review timestamp is invalid",
					item.ID,
				)
			}
		}
	}
	return nil
}

func validatePromotionObservationSet(value PromotionObservationSet) error {
	if value.SchemaVersion != PromotionObservationSchemaVersion ||
		strings.TrimSpace(value.GoldenSetID) == "" ||
		!validSHA256(value.GoldenCorpusSHA256) ||
		!validUUID(value.CaptureID) ||
		!validUUID(value.GenerationID) ||
		!validSHA256(value.ArtifactManifestHash) ||
		strings.TrimSpace(value.ProfileID) == "" ||
		len(value.Cases) == 0 {
		return errors.New("promotion observation set header is invalid")
	}
	if value.ProfileRole != "active" && value.ProfileRole != "candidate" {
		return errors.New("promotion observation profile role is invalid")
	}
	if _, err := parsePromotionTimestamp(value.CapturedAt); err != nil {
		return errors.New("promotion observation capturedAt is invalid")
	}
	if !validUUID(value.HoldoutRun.ID) || value.HoldoutRun.Ordinal < 1 {
		return errors.New("promotion observation Holdout run is invalid")
	}
	if _, err := parsePromotionTimestamp(value.HoldoutRun.ExecutedAt); err != nil {
		return errors.New("promotion observation Holdout timestamp is invalid")
	}

	seen := make(map[string]struct{}, len(value.Cases))
	for _, item := range value.Cases {
		if strings.TrimSpace(item.CaseID) == "" ||
			item.LatencyMilliseconds < 0 ||
			item.ContextTokens < 0 ||
			!validRate(item.AnswerCorrectness) ||
			!validRate(item.Faithfulness) {
			return fmt.Errorf("promotion observation case %q is invalid", item.CaseID)
		}
		if !item.Answered &&
			(item.AnswerCorrectness != 0 ||
				item.Faithfulness != 0 ||
				item.TableExactAnswer) {
			return fmt.Errorf(
				"promotion observation case %q unanswered scores are inconsistent",
				item.CaseID,
			)
		}
		if _, duplicate := seen[item.CaseID]; duplicate {
			return fmt.Errorf("duplicate promotion observation case %q", item.CaseID)
		}
		seen[item.CaseID] = struct{}{}
		if len(item.RetrievedEvidenceIDs) > 50 || len(item.FinalEvidenceIDs) > 10 {
			return fmt.Errorf(
				"promotion observation case %q exceeds rank bounds",
				item.CaseID,
			)
		}
		if hasBlankOrDuplicate(item.RetrievedEvidenceIDs) ||
			hasBlankOrDuplicate(item.FinalEvidenceIDs) ||
			hasBlankOrDuplicate(item.CitationEvidenceIDs) {
			return fmt.Errorf(
				"promotion observation case %q contains blank or duplicate evidence",
				item.CaseID,
			)
		}
	}
	return nil
}

func parsePromotionTimestamp(value string) (time.Time, error) {
	if strings.TrimSpace(value) != value || value == "" {
		return time.Time{}, errors.New("timestamp is blank or padded")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validUUID(value string) bool {
	if len(value) != 36 || strings.ToLower(value) != value {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
				return false
			}
		}
	}
	return true
}
