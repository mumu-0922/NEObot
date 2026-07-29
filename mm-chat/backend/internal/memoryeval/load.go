package memoryeval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	maximumInputBytes          = 64 << 20
	requiredBenchmarkCases     = 500
	requiredCriticalSliceCases = 50
)

var exclusionReasons = map[string]struct{}{
	"cross_user":       {},
	"deleted":          {},
	"irrelevant":       {},
	"out_of_scope":     {},
	"secret":           {},
	"superseded":       {},
	"untrusted_source": {},
}

func DecodeGoldenSet(reader io.Reader) (GoldenSet, error) {
	var value GoldenSet
	if err := decodeStrict(reader, &value); err != nil {
		return GoldenSet{}, fmt.Errorf("decode Memory Golden corpus: %w", err)
	}
	if err := validateGoldenSet(value); err != nil {
		return GoldenSet{}, err
	}
	return value, nil
}

func DecodeObservationSet(reader io.Reader) (ObservationSet, error) {
	var value ObservationSet
	if err := decodeStrict(reader, &value); err != nil {
		return ObservationSet{}, fmt.Errorf("decode Memory observations: %w", err)
	}
	if err := validateObservationSet(value); err != nil {
		return ObservationSet{}, err
	}
	return value, nil
}

func DecodeRegressionCorpus(reader io.Reader) (RegressionCorpus, error) {
	var value RegressionCorpus
	if err := decodeStrict(reader, &value); err != nil {
		return RegressionCorpus{}, fmt.Errorf("decode Memory regression corpus: %w", err)
	}
	if err := validateRegressionCorpus(value); err != nil {
		return RegressionCorpus{}, err
	}
	return value, nil
}

func DecodeRegressionAudit(reader io.Reader) (RegressionAudit, error) {
	var value RegressionAudit
	if err := decodeStrict(reader, &value); err != nil {
		return RegressionAudit{}, fmt.Errorf("decode Memory regression audit: %w", err)
	}
	if err := validateRegressionAudit(value); err != nil {
		return RegressionAudit{}, err
	}
	return value, nil
}

func DecodeRegressionObservationSet(reader io.Reader) (RegressionObservationSet, error) {
	var value RegressionObservationSet
	if err := decodeStrict(reader, &value); err != nil {
		return RegressionObservationSet{}, fmt.Errorf("decode Memory regression observations: %w", err)
	}
	if err := validateRegressionObservationSet(value); err != nil {
		return RegressionObservationSet{}, err
	}
	return value, nil
}

func decodeStrict(reader io.Reader, target any) error {
	if reader == nil {
		return errors.New("input is required")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maximumInputBytes+1))
	if err != nil {
		return err
	}
	if len(body) == 0 || len(body) > maximumInputBytes {
		return errors.New("input size is invalid")
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
	expected := json.Delim('}')
	if delimiter == '[' {
		expected = ']'
	}
	if closing != expected {
		return errors.New("JSON delimiter is not balanced")
	}
	return nil
}

// GoldenContentSHA256 hashes the canonical corpus after clearing only the
// self-referential frozenContentSha256 field.
func GoldenContentSHA256(value GoldenSet) (string, error) {
	value.Lifecycle.FrozenContentSHA256 = ""
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Memory Golden corpus: %w", err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

// RegressionCorpusContentSHA256 hashes the stable regression corpus while
// clearing the self hash and audit content hash. The latter permits a
// fail-closed two-way corpus/audit binding without a hash cycle.
func RegressionCorpusContentSHA256(value RegressionCorpus) (string, error) {
	value.CorpusContentSHA256 = ""
	value.MachineAudit.ContentSHA256 = ""
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Memory regression corpus: %w", err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func RegressionAuditContentSHA256(value RegressionAudit) (string, error) {
	value.ContentSHA256 = ""
	body, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode Memory regression audit: %w", err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func NewFreezeHashReport(value GoldenSet) (FreezeHashReport, error) {
	if err := validateGoldenSet(value); err != nil {
		return FreezeHashReport{}, err
	}
	digest, err := GoldenContentSHA256(value)
	if err != nil {
		return FreezeHashReport{}, err
	}
	return FreezeHashReport{
		SchemaVersion:       FreezeHashSchemaVersion,
		CorpusID:            value.ID,
		State:               value.Lifecycle.State,
		CaseCount:           len(value.Cases),
		FrozenContentSHA256: digest,
		PromotionEligible:   false,
	}, nil
}

func ValidateGoldenAdmission(value GoldenSet) error {
	if err := validateGoldenSet(value); err != nil {
		return err
	}
	return validateGoldenAdmission(value)
}

func ValidateRegressionAdmission(corpus RegressionCorpus, audit RegressionAudit) error {
	if err := validateRegressionCorpus(corpus); err != nil {
		return err
	}
	if err := validateRegressionAudit(audit); err != nil {
		return err
	}
	return validateRegressionAdmission(corpus, audit)
}
