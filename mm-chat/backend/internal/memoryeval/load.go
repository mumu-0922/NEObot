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
