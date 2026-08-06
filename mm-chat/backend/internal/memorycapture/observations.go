package memorycapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/bits"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/strictjson"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const maximumCostBasisBytes = 64 * 1024

func ConfigurationSHA256(config ProfileConfig) (string, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("%w: encode profile configuration", ErrCaptureInvalid)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func CostBasisSHA256(cost CostBasis) (string, error) {
	if cost.Source == "" || cost.EffectiveAt == "" ||
		cost.Baseline.Unit == "" || cost.Candidate.Unit == "" ||
		cost.Baseline.Unit != cost.Candidate.Unit ||
		cost.Baseline.MemoryProviderCostMicrounits != 0 ||
		cost.Candidate.MemoryProviderCostMicrounits == 0 ||
		cost.Baseline.ChatProviderCostMicrounits == 0 ||
		cost.Candidate.ChatProviderCostMicrounits != cost.Baseline.ChatProviderCostMicrounits {
		return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
	}
	switch cost.SchemaVersion {
	case "neo-chat.memory-regression-cost-basis.v1":
		if cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
			cost.ConfiguredCandidateJudgeAuthority != nil ||
			cost.ProviderCostPolicy != "" {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
	case "neo-chat.memory-regression-cost-basis.v2":
		if cost.ProviderCostPolicy != "" || cost.MemoryToolRouteAuthority != nil ||
			cost.ConfiguredCandidateJudgeAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateCloudJudgeCostAuthority(cost, ""); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v3":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.MemoryToolRouteAuthority != nil ||
			cost.ConfiguredCandidateJudgeAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateCloudJudgeCostAuthority(cost, ""); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v4":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil ||
			cost.ConfiguredCandidateJudgeAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateMemoryToolRouteCostAuthority(
			cost,
			MemoryToolRouteProfileAuthority{},
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v5":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil ||
			cost.ConfiguredCandidateJudgeAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateMemoryToolFirstRoundCostAuthority(
			cost,
			MemoryToolRouteProfileAuthority{},
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v6":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			ConfiguredCandidateJudgeProfileAuthority{},
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v7":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			FixedMemoryJudgeAuthority(),
			300,
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v8":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			FixedMemoryJudgeAuthority(),
			600,
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v9":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			FixedMemoryJudgeAuthority(),
			900,
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v10":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			FixedMemoryJudgeAuthority(),
			300,
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v11":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			FixedMemoryJudgeAuthority(),
			900,
		); err != nil {
			return "", err
		}
	case "neo-chat.memory-regression-cost-basis.v12":
		if cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
			cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
			return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
		}
		if err := validateConfiguredCandidateJudgeCostAuthority(
			cost,
			FixedMemoryJudgeAuthority(),
			900,
		); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("%w: cost basis", ErrCaptureInvalid)
	}
	if _, err := time.Parse(time.RFC3339, cost.EffectiveAt); err != nil {
		return "", fmt.Errorf("%w: cost basis timestamp", ErrCaptureInvalid)
	}
	body, err := json.Marshal(cost)
	if err != nil {
		return "", fmt.Errorf("%w: encode cost basis", ErrCaptureInvalid)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func ValidateMemoryToolRouteCostAuthority(
	cost CostBasis,
	authority MemoryToolRouteProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v4" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil ||
		cost.ConfiguredCandidateJudgeAuthority != nil {
		return fmt.Errorf("%w: Memory Tool route cost policy", ErrCaptureInvalid)
	}
	return validateMemoryToolCostAuthority(cost, authority, false)
}

func ValidateMemoryToolFirstRoundCostAuthority(
	cost CostBasis,
	authority MemoryToolRouteProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v5" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil ||
		cost.ConfiguredCandidateJudgeAuthority != nil {
		return fmt.Errorf("%w: Memory first Tool-round cost policy", ErrCaptureInvalid)
	}
	return validateMemoryToolCostAuthority(cost, authority, true)
}

func validateMemoryToolRouteCostAuthority(
	cost CostBasis,
	expected MemoryToolRouteProfileAuthority,
) error {
	return validateMemoryToolCostAuthority(cost, expected, false)
}

func validateMemoryToolFirstRoundCostAuthority(
	cost CostBasis,
	expected MemoryToolRouteProfileAuthority,
) error {
	return validateMemoryToolCostAuthority(cost, expected, true)
}

func validateMemoryToolCostAuthority(
	cost CostBasis,
	expected MemoryToolRouteProfileAuthority,
	firstRound bool,
) error {
	authority := cost.MemoryToolRouteAuthority
	validOutputAuthority := false
	if authority != nil && authority.RequestCount > 0 {
		if firstRound {
			validOutputAuthority =
				authority.MaximumOutputTokens >= uint64(authority.RequestCount) &&
					authority.MaximumOutputTokens%uint64(authority.RequestCount) == 0
		} else {
			validOutputAuthority = authority.MaximumOutputTokens ==
				uint64(authority.RequestCount)*usermemory.HybridMemoryToolMaximumOutputTokens
		}
	}
	if authority == nil || strings.TrimSpace(authority.ProviderID) == "" ||
		authority.ProviderID != strings.TrimSpace(authority.ProviderID) ||
		strings.TrimSpace(authority.ProviderType) == "" ||
		authority.ProviderType != strings.TrimSpace(authority.ProviderType) ||
		len(authority.BaseURLSHA256) != 64 ||
		strings.TrimSpace(authority.ModelID) == "" ||
		authority.ModelID != strings.TrimSpace(authority.ModelID) ||
		authority.RequestCount != 300 ||
		authority.MaximumInputTokens < uint64(authority.RequestCount) ||
		!validOutputAuthority {
		return fmt.Errorf("%w: Memory Tool cost authority", ErrCaptureInvalid)
	}
	if _, err := hex.DecodeString(authority.BaseURLSHA256); err != nil {
		return fmt.Errorf("%w: Memory Tool base URL hash", ErrCaptureInvalid)
	}
	if expected.ProviderID != "" &&
		(authority.ProviderID != expected.ProviderID ||
			authority.ProviderType != expected.ProviderType ||
			authority.BaseURLSHA256 != expected.BaseURLSHA256 ||
			authority.ModelID != expected.ModelID) {
		return fmt.Errorf("%w: Memory Tool Provider authority", ErrCaptureInvalid)
	}
	inputCost, ok := tokenCostCeiling(
		authority.MaximumInputTokens,
		authority.InputMicrounitsPerMillionTokens,
	)
	if !ok {
		return fmt.Errorf("%w: Memory Tool input cost overflow", ErrCaptureInvalid)
	}
	outputCost, ok := tokenCostCeiling(
		authority.MaximumOutputTokens,
		authority.OutputMicrounitsPerMillionTokens,
	)
	if !ok || inputCost > ^uint64(0)-outputCost {
		return fmt.Errorf("%w: Memory Tool output cost overflow", ErrCaptureInvalid)
	}
	maximum := inputCost + outputCost
	if authority.MaximumCostMicrounits != maximum ||
		cost.Candidate.MemoryProviderCostMicrounits < maximum {
		return fmt.Errorf("%w: Memory Tool cost total", ErrCaptureInvalid)
	}
	return nil
}

func ValidateCloudJudgeCostAuthority(cost CostBasis, modelID string) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v2" &&
		cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v3" {
		return fmt.Errorf("%w: cloud-judge cost basis version", ErrCaptureInvalid)
	}
	if cost.SchemaVersion == "neo-chat.memory-regression-cost-basis.v2" &&
		cost.ProviderCostPolicy != "" {
		return fmt.Errorf("%w: cloud-judge cost policy", ErrCaptureInvalid)
	}
	if cost.SchemaVersion == "neo-chat.memory-regression-cost-basis.v3" &&
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 {
		return fmt.Errorf("%w: cloud-judge cost policy", ErrCaptureInvalid)
	}
	if cost.MemoryToolRouteAuthority != nil ||
		cost.ConfiguredCandidateJudgeAuthority != nil {
		return fmt.Errorf("%w: cloud-judge cost authority", ErrCaptureInvalid)
	}
	return validateCloudJudgeCostAuthority(cost, modelID)
}

func ValidateConfiguredCandidateJudgeCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v6" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil {
		return fmt.Errorf("%w: configured candidate-judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority)
}

func ValidateFixedMemoryJudgeCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v7" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: fixed Memory Judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority, 300)
}

func ValidateAccuracyFirstMemoryJudgeCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v8" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: accuracy-first Memory Judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority, 600)
}

func ValidateTransportStableMemoryJudgeCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v9" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: transport-stable Memory Judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority, 900)
}

func ValidateProductionMemoryJudgeValidationCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v10" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: production Memory Judge Validation cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority, 300)
}

func ValidateNegativePolicyGuardMemoryJudgeCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v11" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: negative-policy-guard Memory Judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority, 900)
}

func ValidateBufferedMemoryJudgeCostAuthority(
	cost CostBasis,
	authority ConfiguredCandidateJudgeProfileAuthority,
) error {
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v12" ||
		cost.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		cost.CloudJudgeAuthority != nil || cost.MemoryToolRouteAuthority != nil ||
		!validFixedMemoryJudgeAuthority(authority) {
		return fmt.Errorf("%w: buffered Memory Judge cost policy", ErrCaptureInvalid)
	}
	return validateConfiguredCandidateJudgeCostAuthority(cost, authority, 900)
}

func validateConfiguredCandidateJudgeCostAuthority(
	cost CostBasis,
	expected ConfiguredCandidateJudgeProfileAuthority,
	expectedRequestCount ...int,
) error {
	requestCount := 300
	if len(expectedRequestCount) > 1 {
		return fmt.Errorf("%w: configured candidate-judge cost authority", ErrCaptureInvalid)
	}
	if len(expectedRequestCount) == 1 {
		requestCount = expectedRequestCount[0]
	}
	authority := cost.ConfiguredCandidateJudgeAuthority
	if authority == nil || strings.TrimSpace(authority.ProviderID) == "" ||
		authority.ProviderID != strings.TrimSpace(authority.ProviderID) ||
		strings.TrimSpace(authority.ProviderType) == "" ||
		authority.ProviderType != strings.TrimSpace(authority.ProviderType) ||
		len(authority.BaseURLSHA256) != 64 ||
		strings.TrimSpace(authority.ModelID) == "" ||
		authority.ModelID != strings.TrimSpace(authority.ModelID) ||
		authority.RequestCount != requestCount ||
		authority.MaximumInputTokens < uint64(authority.RequestCount) ||
		authority.MaximumOutputTokens != uint64(authority.RequestCount)*
			usermemory.HybridCandidateJudgeMaximumOutputTokens {
		return fmt.Errorf("%w: configured candidate-judge cost authority", ErrCaptureInvalid)
	}
	if _, err := hex.DecodeString(authority.BaseURLSHA256); err != nil {
		return fmt.Errorf("%w: configured candidate-judge base URL hash", ErrCaptureInvalid)
	}
	if expected.ProviderID != "" &&
		(authority.ProviderID != expected.ProviderID ||
			authority.ProviderType != expected.ProviderType ||
			authority.BaseURLSHA256 != expected.BaseURLSHA256 ||
			authority.ModelID != expected.ModelID) {
		return fmt.Errorf("%w: configured candidate-judge Provider authority", ErrCaptureInvalid)
	}
	inputCost, ok := tokenCostCeiling(
		authority.MaximumInputTokens,
		authority.InputMicrounitsPerMillionTokens,
	)
	if !ok {
		return fmt.Errorf("%w: configured candidate-judge input cost overflow", ErrCaptureInvalid)
	}
	outputCost, ok := tokenCostCeiling(
		authority.MaximumOutputTokens,
		authority.OutputMicrounitsPerMillionTokens,
	)
	if !ok || inputCost > ^uint64(0)-outputCost {
		return fmt.Errorf("%w: configured candidate-judge output cost overflow", ErrCaptureInvalid)
	}
	maximum := inputCost + outputCost
	if authority.MaximumCostMicrounits != maximum ||
		cost.Candidate.MemoryProviderCostMicrounits < maximum {
		return fmt.Errorf("%w: configured candidate-judge cost total", ErrCaptureInvalid)
	}
	return nil
}

func validateCloudJudgeCostAuthority(cost CostBasis, modelID string) error {
	authority := cost.CloudJudgeAuthority
	if authority == nil || strings.TrimSpace(authority.ModelID) == "" ||
		authority.ModelID != strings.TrimSpace(authority.ModelID) ||
		(modelID != "" && authority.ModelID != modelID) ||
		authority.RequestCount != 300 ||
		authority.MaximumInputTokens < uint64(authority.RequestCount) ||
		authority.MaximumOutputTokens != uint64(authority.RequestCount)*
			usermemory.HybridCandidateJudgeMaximumOutputTokens {
		return fmt.Errorf("%w: cloud-judge cost authority", ErrCaptureInvalid)
	}
	inputCost, ok := tokenCostCeiling(
		authority.MaximumInputTokens,
		authority.InputMicrounitsPerMillionTokens,
	)
	if !ok {
		return fmt.Errorf("%w: cloud-judge input cost overflow", ErrCaptureInvalid)
	}
	outputCost, ok := tokenCostCeiling(
		authority.MaximumOutputTokens,
		authority.OutputMicrounitsPerMillionTokens,
	)
	if !ok || inputCost > ^uint64(0)-outputCost {
		return fmt.Errorf("%w: cloud-judge output cost overflow", ErrCaptureInvalid)
	}
	maximum := inputCost + outputCost
	if authority.MaximumCostMicrounits != maximum ||
		cost.Candidate.MemoryProviderCostMicrounits < maximum {
		return fmt.Errorf("%w: cloud-judge cost total", ErrCaptureInvalid)
	}
	return nil
}

func tokenCostCeiling(tokens uint64, microunitsPerMillion uint64) (uint64, bool) {
	high, low := bits.Mul64(tokens, microunitsPerMillion)
	if high != 0 || low > ^uint64(0)-999_999 {
		return 0, false
	}
	return (low + 999_999) / 1_000_000, true
}

// DecodeCostBasis rejects duplicate keys, unknown fields, trailing values,
// oversized inputs, and invalid cost authority before Provider work. Exact
// zero cloud-judge rates are valid when the versioned Provider rate card makes
// the fixed model free; the enclosing candidate Memory cost remains positive.
func DecodeCostBasis(body []byte) (CostBasis, string, error) {
	var cost CostBasis
	if err := strictjson.Decode(body, maximumCostBasisBytes, &cost); err != nil {
		return CostBasis{}, "", fmt.Errorf("%w: decode cost basis", ErrCaptureInvalid)
	}
	digest, err := CostBasisSHA256(cost)
	if err != nil {
		return CostBasis{}, "", err
	}
	return cost, digest, nil
}

// RegressionCaptureTimestamp satisfies the deterministic machine-audit time
// floor. The regression generator uses a fixed auditedAt for byte replay, so
// a protocol run earlier on that wall-clock day must not fabricate a human
// review claim or weaken evaluator admission.
func RegressionCaptureTimestamp(pool memoryauthor.RegressionPool, now time.Time) (time.Time, error) {
	if now.IsZero() {
		return time.Time{}, ErrCaptureInvalid
	}
	auditedAt, err := time.Parse(time.RFC3339, pool.Audit.AuditedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: regression audit timestamp", ErrCaptureInvalid)
	}
	now = now.UTC()
	if now.Before(auditedAt) {
		return auditedAt, nil
	}
	return now, nil
}

// AssembleRegressionObservations binds a complete ordered capture to the
// admitted machine-regression corpus and reuses the strict observation decoder
// as the final constructor invariant.
func AssembleRegressionObservations(
	pool memoryauthor.RegressionPool,
	capturedAt time.Time,
	captureID string,
	profile CapturedProfile,
) (memoryeval.RegressionObservationSet, []byte, error) {
	if capturedAt.IsZero() || captureID == "" || len(profile.Cases) != len(pool.Corpus.Cases) {
		return memoryeval.RegressionObservationSet{}, nil, ErrCaptureInvalid
	}
	ordered := make([]memoryeval.CaseObservation, len(pool.Corpus.Cases))
	byID := make(map[string]memoryeval.CaseObservation, len(profile.Cases))
	for _, item := range profile.Cases {
		if _, duplicate := byID[item.CaseID]; duplicate {
			return memoryeval.RegressionObservationSet{}, nil, fmt.Errorf("%w: duplicate case", ErrCaptureInvalid)
		}
		byID[item.CaseID] = item
	}
	for index, item := range pool.Corpus.Cases {
		observed, ok := byID[item.ID]
		if !ok {
			return memoryeval.RegressionObservationSet{}, nil, fmt.Errorf("%w: missing case", ErrCaptureInvalid)
		}
		ordered[index] = observed
	}
	value := memoryeval.RegressionObservationSet{
		SchemaVersion: memoryeval.RegressionObservationSchemaVersion,
		CorpusID:      pool.Corpus.ID, CorpusContentSHA256: pool.Corpus.CorpusContentSHA256,
		AuditContentSHA256:    pool.Audit.ContentSHA256,
		FixtureManifestSHA256: pool.Fixtures.ContentSHA256,
		CapturedAt:            capturedAt.UTC().Format(time.RFC3339), CaptureID: captureID,
		Profile: profile.Profile, Costs: profile.Costs, Cases: ordered,
	}
	body, err := json.Marshal(value)
	if err != nil {
		return memoryeval.RegressionObservationSet{}, nil, fmt.Errorf("%w: encode observations", ErrCaptureInvalid)
	}
	body = append(body, '\n')
	decoded, err := memoryeval.DecodeRegressionObservationSet(bytes.NewReader(body))
	if err != nil {
		return memoryeval.RegressionObservationSet{}, nil, fmt.Errorf("%w: validate observations: %v", ErrCaptureInvalid, err)
	}
	return decoded, body, nil
}
