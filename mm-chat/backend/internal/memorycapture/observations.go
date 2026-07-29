package memorycapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/strictjson"
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
	if cost.SchemaVersion != "neo-chat.memory-regression-cost-basis.v1" ||
		cost.Source == "" || cost.EffectiveAt == "" ||
		cost.Baseline.Unit == "" || cost.Candidate.Unit == "" ||
		cost.Baseline.Unit != cost.Candidate.Unit ||
		cost.Baseline.MemoryProviderCostMicrounits != 0 ||
		cost.Candidate.MemoryProviderCostMicrounits == 0 ||
		cost.Baseline.ChatProviderCostMicrounits == 0 ||
		cost.Candidate.ChatProviderCostMicrounits != cost.Baseline.ChatProviderCostMicrounits {
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

// DecodeCostBasis rejects duplicate keys, unknown fields, trailing values,
// oversized inputs, and invalid/zero-cost authority before Provider work.
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
