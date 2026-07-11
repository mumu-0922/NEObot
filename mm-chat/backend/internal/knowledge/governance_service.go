package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maximumGovernanceAliasBytes = 128
	maximumGovernanceValueBytes = 4096
	maximumGovernanceListItems  = 32
)

var governanceAliasPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var governanceModelVersionPattern = regexp.MustCompile(`^v?[0-9][a-z0-9._-]*$`)
var governanceDataTypePattern = regexp.MustCompile(
	`^(\*|[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*)$`,
)

var governancePolicyValues = map[string]map[string]struct{}{
	"region":            {"global": {}},
	"retention policy":  {"none": {}},
	"deletion contract": {"delete": {}},
	"training use":      {"disabled": {}},
}

type GovernanceService struct{ repo GovernanceRepository }

func NewGovernanceService(repo GovernanceRepository) *GovernanceService {
	return &GovernanceService{repo: repo}
}

func (s *GovernanceService) Apply(ctx context.Context, manifest GovernanceManifest) (ProcessorGovernanceHead, error) {
	if s == nil || s.repo == nil {
		return ProcessorGovernanceHead{}, ErrDatabaseRequired
	}
	normalized, hash, err := normalizeGovernanceManifest(manifest)
	if err != nil {
		return ProcessorGovernanceHead{}, err
	}
	return s.repo.ApplyGovernance(ctx, normalized, hash)
}

func (s *GovernanceService) Disable(ctx context.Context, processor, endpointID string) (ProcessorGovernanceHead, error) {
	if s == nil || s.repo == nil {
		return ProcessorGovernanceHead{}, ErrDatabaseRequired
	}
	processor, err := normalizeGovernanceAlias(processor, "processor")
	if err != nil {
		return ProcessorGovernanceHead{}, err
	}
	endpointID, err = normalizeGovernanceAlias(endpointID, "endpoint id")
	if err != nil {
		return ProcessorGovernanceHead{}, err
	}
	return s.repo.DisableGovernance(ctx, processor, endpointID)
}

func normalizeGovernanceManifest(input GovernanceManifest) (GovernanceManifest, string, error) {
	var err error
	input.Processor, err = normalizeGovernanceAlias(input.Processor, "processor")
	if err != nil {
		return input, "", err
	}
	input.EndpointID, err = normalizeGovernanceAlias(input.EndpointID, "endpoint id")
	if err != nil {
		return input, "", err
	}
	for label, value := range map[string]*string{
		"model API version": &input.ModelAPIVersion,
	} {
		*value = strings.TrimSpace(*value)
		if *value == "" || !utf8.ValidString(*value) ||
			len(*value) > maximumGovernanceValueBytes || !governanceModelVersionPattern.MatchString(*value) {
			return input, "", fmt.Errorf("invalid %s", label)
		}
	}
	for label, value := range map[string]*string{
		"region":            &input.Region,
		"retention policy":  &input.RetentionPolicy,
		"deletion contract": &input.DeletionContract,
		"training use":      &input.TrainingUse,
	} {
		*value = strings.TrimSpace(*value)
		if _, allowed := governancePolicyValues[label][*value]; !allowed {
			return input, "", fmt.Errorf("invalid %s", label)
		}
	}
	input.AllowedPurposes, err = normalizeGovernanceList(input.AllowedPurposes, true)
	if err != nil {
		return input, "", err
	}
	input.AllowedDataTypes, err = normalizeGovernanceList(input.AllowedDataTypes, false)
	if err != nil {
		return input, "", err
	}
	canonical := struct {
		SchemaVersion    int      `json:"schemaVersion"`
		Processor        string   `json:"processor"`
		EndpointID       string   `json:"endpointId"`
		ModelAPIVersion  string   `json:"modelApiVersion"`
		AllowedPurposes  []string `json:"allowedPurposes"`
		AllowedDataTypes []string `json:"allowedDataTypes"`
		Region           string   `json:"region"`
		RetentionPolicy  string   `json:"retentionPolicy"`
		DeletionContract string   `json:"deletionContract"`
		TrainingUse      string   `json:"trainingUse"`
	}{1, input.Processor, input.EndpointID, input.ModelAPIVersion,
		input.AllowedPurposes, input.AllowedDataTypes, input.Region,
		input.RetentionPolicy, input.DeletionContract, input.TrainingUse}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return input, "", fmt.Errorf("marshal governance manifest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return input, hex.EncodeToString(digest[:]), nil
}

func normalizeGovernanceAlias(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > maximumGovernanceAliasBytes || !governanceAliasPattern.MatchString(value) {
		return "", fmt.Errorf("invalid %s", label)
	}
	return value, nil
}

func normalizeGovernanceList(values []string, purposes bool) ([]string, error) {
	if len(values) == 0 || len(values) > maximumGovernanceListItems {
		return nil, errorsGovernanceList(purposes)
	}
	allowed := map[string]bool{"parse": true, "passage_embedding": true, "query_embedding": true, "rerank": true, "answer": true}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !utf8.ValidString(value) || len(value) > 256 ||
			(purposes && !allowed[value]) || (!purposes && !governanceDataTypePattern.MatchString(value)) {
			return nil, errorsGovernanceList(purposes)
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func errorsGovernanceList(purposes bool) error {
	if purposes {
		return fmt.Errorf("invalid allowed purposes")
	}
	return fmt.Errorf("invalid allowed data types")
}
