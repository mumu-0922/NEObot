package memorycapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const RunManifestSchemaVersion = "neo-chat.memory-regression-native-run.v1"

type RunManifest struct {
	SchemaVersion     string                `json:"schemaVersion"`
	RunID             string                `json:"runId"`
	CaptureID         string                `json:"captureId"`
	CorpusClass       string                `json:"corpusClass"`
	AdmissionMode     string                `json:"admissionMode"`
	PromotionEligible bool                  `json:"promotionEligible"`
	ProviderMode      string                `json:"providerMode"`
	StartedAt         string                `json:"startedAt"`
	CompletedAt       string                `json:"completedAt"`
	CostBasisSHA256   string                `json:"costBasisSha256"`
	Inputs            RunInputHashes        `json:"inputs"`
	Profiles          []RunProfileManifest  `json:"profiles"`
	Artifacts         []RunArtifactManifest `json:"artifacts"`
}

type RunInputHashes struct {
	FixtureRawSHA256  string `json:"fixtureRawSha256"`
	CorpusRawSHA256   string `json:"corpusRawSha256"`
	AuditRawSHA256    string `json:"auditRawSha256"`
	ManifestRawSHA256 string `json:"manifestRawSha256"`
}

type RunProfileManifest struct {
	Role                string `json:"role"`
	ProfileID           string `json:"profileId"`
	ConfigurationSHA256 string `json:"configurationSha256"`
	Passed              bool   `json:"passed"`
}

type RunArtifactManifest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

func BuildRunManifest(
	runID string,
	captureID string,
	providerMode string,
	startedAt time.Time,
	completedAt time.Time,
	protected ProtectedRegression,
	costBasisSHA256 string,
	profileHashes ProfileHashes,
	baseline memoryeval.RegressionReport,
	candidate memoryeval.RegressionReport,
	artifacts []Artifact,
) (RunManifest, []byte, error) {
	if !runIDPattern.MatchString(runID) || captureID == "" ||
		startedAt.IsZero() || completedAt.Before(startedAt) ||
		len(costBasisSHA256) != 64 || len(artifacts) == 0 {
		return RunManifest{}, nil, ErrCaptureInvalid
	}
	if _, err := candidateProfileID(providerMode); err != nil {
		return RunManifest{}, nil, err
	}
	profiles := []RunProfileManifest{
		{Role: "baseline", ProfileID: baseline.Profile.ProfileID,
			ConfigurationSHA256: profileHashes.Baseline, Passed: baseline.Passed},
		{Role: "candidate", ProfileID: candidate.Profile.ProfileID,
			ConfigurationSHA256: profileHashes.Candidate, Passed: candidate.Passed},
	}
	if profileHashes.BaselineProfileID != profiles[0].ProfileID ||
		profileHashes.CandidateProfileID != profiles[1].ProfileID ||
		len(profileHashes.Baseline) != 64 || len(profileHashes.Candidate) != 64 {
		return RunManifest{}, nil, ErrCaptureInvalid
	}
	artifactManifest := make([]RunArtifactManifest, len(artifacts))
	seen := make(map[string]struct{}, len(artifacts))
	for index, artifact := range artifacts {
		if !validArtifactName(artifact.Name) || len(artifact.Body) == 0 {
			return RunManifest{}, nil, ErrCaptureInvalid
		}
		if _, duplicate := seen[artifact.Name]; duplicate {
			return RunManifest{}, nil, ErrCaptureInvalid
		}
		seen[artifact.Name] = struct{}{}
		digest := sha256.Sum256(artifact.Body)
		artifactManifest[index] = RunArtifactManifest{
			Name: artifact.Name, SHA256: hex.EncodeToString(digest[:]), Bytes: len(artifact.Body),
		}
	}
	sort.Slice(artifactManifest, func(i, j int) bool {
		return artifactManifest[i].Name < artifactManifest[j].Name
	})
	manifest := RunManifest{
		SchemaVersion: RunManifestSchemaVersion, RunID: runID, CaptureID: captureID,
		CorpusClass:   memoryeval.RegressionCorpusClass,
		AdmissionMode: memoryeval.RegressionAdmissionMode, PromotionEligible: false,
		ProviderMode:    providerMode,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
		CompletedAt:     completedAt.UTC().Format(time.RFC3339),
		CostBasisSHA256: costBasisSHA256,
		Inputs: RunInputHashes{
			FixtureRawSHA256:  protected.FixtureRawSHA256,
			CorpusRawSHA256:   protected.CorpusRawSHA256,
			AuditRawSHA256:    protected.AuditRawSHA256,
			ManifestRawSHA256: protected.ManifestRawSHA256,
		},
		Profiles: profiles, Artifacts: artifactManifest,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		return RunManifest{}, nil, fmt.Errorf("%w: encode run manifest", ErrCaptureInvalid)
	}
	return manifest, append(body, '\n'), nil
}

// VerifyRetainedArtifactsLeakFree rejects exact synthetic queries, Memory
// plaintext, and supplied live credential bytes in the retained content-free
// bundle. Errors never echo the matched value.
func VerifyRetainedArtifactsLeakFree(
	pool memoryauthor.RegressionPool,
	artifacts []Artifact,
	credential []byte,
) error {
	forbidden := make([][]byte, 0, len(pool.Corpus.Cases)+len(pool.Fixtures.Fixtures)*2+1)
	for _, item := range pool.Corpus.Cases {
		forbidden = appendForbiddenRepresentations(forbidden, item.Query)
	}
	for _, fixture := range pool.Fixtures.Fixtures {
		for _, memory := range fixture.Memories {
			forbidden = appendForbiddenRepresentations(forbidden, memory.CanonicalContent)
		}
	}
	if len(credential) > 0 {
		forbidden = append(forbidden, append([]byte(nil), credential...))
	}
	for _, artifact := range artifacts {
		for _, value := range forbidden {
			if len(value) >= 8 && bytes.Contains(artifact.Body, value) {
				return fmt.Errorf("%w: retained artifact contains protected plaintext", ErrCaptureStateConflict)
			}
		}
	}
	return nil
}

func appendForbiddenRepresentations(values [][]byte, text string) [][]byte {
	text = strings.TrimSpace(text)
	if len([]byte(text)) < 8 {
		return values
	}
	values = append(values, []byte(text))
	encoded, err := json.Marshal(text)
	if err == nil && len(encoded) >= 2 {
		values = append(values, encoded[1:len(encoded)-1])
	}
	return values
}
