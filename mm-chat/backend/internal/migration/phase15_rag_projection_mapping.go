package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const phase15GovernanceMapSetting = "mm_chat.phase15_governance_map"

var (
	phase15UUIDPattern = regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`,
	)
	phase15HashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Phase15GovernanceMapping is release evidence used only while migration 010
// is running. It is never persisted by the Go runner.
type Phase15GovernanceMapping struct {
	Profiles []Phase15GovernanceProfileMapping `json:"profiles"`
	Heads    []Phase15GovernanceHeadMapping    `json:"heads"`
}

type Phase15GovernanceProfileMapping struct {
	ProfileID           string `json:"profileId"`
	ModelID             string `json:"modelId"`
	ProfileContractHash string `json:"profileContractHash"`
}

type Phase15GovernanceHeadMapping struct {
	Processor  string `json:"processor"`
	EndpointID string `json:"endpointId"`
	ModelID    string `json:"modelId"`
}

// ParsePhase15GovernanceMapping parses the mapping with a closed JSON shape.
// Canonical UUIDs, hashes, strings, and duplicate keys are rejected before a
// database connection is opened.
func ParsePhase15GovernanceMapping(data []byte) (Phase15GovernanceMapping, error) {
	var mapping Phase15GovernanceMapping
	if err := rejectDuplicatePhase15JSONKeys(data); err != nil {
		return Phase15GovernanceMapping{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&mapping); err != nil {
		return Phase15GovernanceMapping{}, fmt.Errorf("invalid Phase 15 governance mapping JSON: %w", err)
	}
	if err := ensurePhase15JSONEOF(decoder); err != nil {
		return Phase15GovernanceMapping{}, err
	}
	if mapping.Profiles == nil || mapping.Heads == nil {
		return Phase15GovernanceMapping{}, errors.New(
			"invalid Phase 15 governance mapping JSON: profiles and heads must be arrays",
		)
	}
	if err := validatePhase15GovernanceMapping(mapping); err != nil {
		return Phase15GovernanceMapping{}, err
	}

	// Canonical ordering makes the transaction-local value deterministic while
	// preserving no file content in logs or persistent migration metadata.
	sort.Slice(mapping.Profiles, func(i, j int) bool {
		return mapping.Profiles[i].ProfileID < mapping.Profiles[j].ProfileID
	})
	sort.Slice(mapping.Heads, func(i, j int) bool {
		left, right := mapping.Heads[i], mapping.Heads[j]
		if left.Processor != right.Processor {
			return left.Processor < right.Processor
		}
		return left.EndpointID < right.EndpointID
	})
	return mapping, nil
}

func rejectDuplicatePhase15JSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanPhase15JSONValue(decoder); err != nil {
		return fmt.Errorf("invalid Phase 15 governance mapping JSON: %w", err)
	}
	return nil
}

func scanPhase15JSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return keyErr
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, duplicate := keys[key]; duplicate {
				return errors.New("duplicate object key")
			}
			keys[key] = struct{}{}
			if valueErr := scanPhase15JSONValue(decoder); valueErr != nil {
				return valueErr
			}
		}
	case '[':
		for decoder.More() {
			if valueErr := scanPhase15JSONValue(decoder); valueErr != nil {
				return valueErr
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	want := json.Delim('}')
	if delimiter == '[' {
		want = ']'
	}
	if closing != want {
		return errors.New("mismatched JSON delimiter")
	}
	return nil
}

func ensurePhase15JSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid Phase 15 governance mapping JSON: %w", err)
	}
	return errors.New("invalid Phase 15 governance mapping JSON: multiple JSON values")
}

func validatePhase15GovernanceMapping(mapping Phase15GovernanceMapping) error {
	profileIDs := make(map[string]struct{}, len(mapping.Profiles))
	for i, profile := range mapping.Profiles {
		if !phase15UUIDPattern.MatchString(profile.ProfileID) {
			return fmt.Errorf("invalid Phase 15 governance mapping: profiles[%d].profileId is not a canonical UUID", i)
		}
		if err := validatePhase15CanonicalText(profile.ModelID); err != nil {
			return fmt.Errorf("invalid Phase 15 governance mapping: profiles[%d].modelId %w", i, err)
		}
		if !phase15HashPattern.MatchString(profile.ProfileContractHash) {
			return fmt.Errorf("invalid Phase 15 governance mapping: profiles[%d].profileContractHash must be 64 lowercase hex characters", i)
		}
		if _, duplicate := profileIDs[profile.ProfileID]; duplicate {
			return fmt.Errorf("invalid Phase 15 governance mapping: duplicate profileId %q", profile.ProfileID)
		}
		profileIDs[profile.ProfileID] = struct{}{}
	}

	headKeys := make(map[string]struct{}, len(mapping.Heads))
	for i, head := range mapping.Heads {
		for field, value := range map[string]string{
			"processor":  head.Processor,
			"endpointId": head.EndpointID,
			"modelId":    head.ModelID,
		} {
			if err := validatePhase15CanonicalText(value); err != nil {
				return fmt.Errorf("invalid Phase 15 governance mapping: heads[%d].%s %w", i, field, err)
			}
		}
		key := head.Processor + "\x00" + head.EndpointID
		if _, duplicate := headKeys[key]; duplicate {
			return fmt.Errorf(
				"invalid Phase 15 governance mapping: duplicate head for processor %q and endpointId %q",
				head.Processor,
				head.EndpointID,
			)
		}
		headKeys[key] = struct{}{}
	}
	return nil
}

func validatePhase15CanonicalText(value string) error {
	if value == "" || strings.TrimSpace(value) == "" {
		return errors.New("must not be blank")
	}
	if strings.TrimSpace(value) != value {
		return errors.New("must not have leading or trailing whitespace")
	}
	if strings.ContainsRune(value, '\x00') {
		return errors.New("must not contain NUL")
	}
	return nil
}

func canonicalPhase15GovernanceMapping(mapping Phase15GovernanceMapping) (string, error) {
	if mapping.Profiles == nil {
		mapping.Profiles = []Phase15GovernanceProfileMapping{}
	}
	if mapping.Heads == nil {
		mapping.Heads = []Phase15GovernanceHeadMapping{}
	}
	if err := validatePhase15GovernanceMapping(mapping); err != nil {
		return "", err
	}
	// Copy into non-nil slices so a valid zero-value mapping remains JSON
	// arrays after canonicalization instead of regressing back to JSON null.
	mapping.Profiles = append([]Phase15GovernanceProfileMapping{}, mapping.Profiles...)
	mapping.Heads = append([]Phase15GovernanceHeadMapping{}, mapping.Heads...)
	sort.Slice(mapping.Profiles, func(i, j int) bool {
		return mapping.Profiles[i].ProfileID < mapping.Profiles[j].ProfileID
	})
	sort.Slice(mapping.Heads, func(i, j int) bool {
		left, right := mapping.Heads[i], mapping.Heads[j]
		if left.Processor != right.Processor {
			return left.Processor < right.Processor
		}
		return left.EndpointID < right.EndpointID
	})
	encoded, err := json.Marshal(mapping)
	if err != nil {
		return "", fmt.Errorf("encode Phase 15 governance mapping: %w", err)
	}
	return string(encoded), nil
}
