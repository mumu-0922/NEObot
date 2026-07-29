package memoryauthor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func ValidateRegressionRoot(root, repositoryRoot string) (string, error) {
	validated, err := ValidateFormalRoot(root, repositoryRoot)
	if err != nil {
		return "", err
	}
	if !strings.Contains(strings.ToLower(filepath.Base(validated)), "regression") {
		return "", errors.New("regression root must use an explicit regression version name")
	}
	return validated, nil
}

func PublishRegression(root string, pool RegressionPool) error {
	if err := ValidateRegressionPool(pool); err != nil {
		return err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || root == "" {
		return errors.New("regression output root is invalid")
	}
	if _, err := os.Lstat(root); err == nil {
		return errors.New("regression output root already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect regression output root: %w", err)
	}
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return errors.New("create regression output parent")
	}
	if err := rejectSymlinkComponents(parent); err != nil {
		return err
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return fmt.Errorf("create regression output root: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.WriteFile(filepath.Join(root, ".generation-incomplete"), []byte("incomplete\n"), 0o600)
		}
	}()
	for _, artifact := range []struct {
		name string
		body []byte
	}{
		{name: RegressionFixtureFile, body: pool.FixtureJSON},
		{name: RegressionCorpusFile, body: pool.CorpusJSON},
		{name: RegressionAuditFile, body: pool.AuditJSON},
		{name: RegressionManifestFile, body: pool.ManifestJSON},
	} {
		if err := writeExclusiveBytes(filepath.Join(root, artifact.name), artifact.body); err != nil {
			return err
		}
	}
	if err := syncDirectory(root); err != nil {
		return err
	}
	complete = true
	return nil
}

func LoadRegression(root string) (RegressionPool, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if err := validateSecureDirectory(root); err != nil {
		return RegressionPool{}, fmt.Errorf("validate regression root: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".generation-incomplete")); err == nil {
		return RegressionPool{}, errors.New("regression generation is incomplete and cannot be resumed in place")
	} else if !errors.Is(err, os.ErrNotExist) {
		return RegressionPool{}, errors.New("inspect regression generation marker")
	}
	fixtureBody, err := readSecureArtifact(filepath.Join(root, RegressionFixtureFile))
	if err != nil {
		return RegressionPool{}, fmt.Errorf("read regression fixtures: %w", err)
	}
	corpusBody, err := readSecureArtifact(filepath.Join(root, RegressionCorpusFile))
	if err != nil {
		return RegressionPool{}, fmt.Errorf("read regression corpus: %w", err)
	}
	auditBody, err := readSecureArtifact(filepath.Join(root, RegressionAuditFile))
	if err != nil {
		return RegressionPool{}, fmt.Errorf("read regression audit: %w", err)
	}
	manifestBody, err := readSecureArtifact(filepath.Join(root, RegressionManifestFile))
	if err != nil {
		return RegressionPool{}, fmt.Errorf("read regression manifest: %w", err)
	}
	fixtures, err := DecodeRegressionFixtureManifest(bytes.NewReader(fixtureBody))
	if err != nil {
		return RegressionPool{}, err
	}
	corpus, err := memoryeval.DecodeRegressionCorpus(bytes.NewReader(corpusBody))
	if err != nil {
		return RegressionPool{}, err
	}
	audit, err := memoryeval.DecodeRegressionAudit(bytes.NewReader(auditBody))
	if err != nil {
		return RegressionPool{}, err
	}
	manifest, err := DecodeRegressionManifest(bytes.NewReader(manifestBody))
	if err != nil {
		return RegressionPool{}, err
	}
	pool := RegressionPool{
		Fixtures: fixtures, Corpus: corpus, Audit: audit, Manifest: manifest,
		FixtureJSON: fixtureBody, CorpusJSON: corpusBody, AuditJSON: auditBody,
		ManifestJSON: manifestBody,
	}
	if err := ValidateRegressionPool(pool); err != nil {
		return RegressionPool{}, err
	}
	return pool, nil
}

func CurrentRegressionStatus(root string) (RegressionStatus, error) {
	pool, err := LoadRegression(root)
	if err != nil {
		return RegressionStatus{}, err
	}
	return regressionStatus(pool), nil
}

func VerifyRegression(root string) (RegressionStatus, error) {
	actual, err := LoadRegression(root)
	if err != nil {
		return RegressionStatus{}, err
	}
	expected, err := GenerateRegression()
	if err != nil {
		return RegressionStatus{}, err
	}
	if !bytes.Equal(actual.FixtureJSON, expected.FixtureJSON) ||
		!bytes.Equal(actual.CorpusJSON, expected.CorpusJSON) ||
		!bytes.Equal(actual.AuditJSON, expected.AuditJSON) ||
		!bytes.Equal(actual.ManifestJSON, expected.ManifestJSON) {
		return RegressionStatus{}, errors.New("regression pool does not byte-match the fixed generator profile")
	}
	return regressionStatus(actual), nil
}

func regressionStatus(pool RegressionPool) RegressionStatus {
	manifest := pool.Manifest
	return RegressionStatus{
		SchemaVersion:        RegressionStatusSchemaVersion,
		CorpusClass:          manifest.CorpusClass,
		AdmissionMode:        manifest.AdmissionMode,
		PromotionEligible:    false,
		Profile:              manifest.Generator.Profile,
		GeneratorVersion:     manifest.Generator.Version,
		CaseCount:            manifest.CaseCount,
		SplitCounts:          manifest.SplitCounts,
		LanguageCounts:       manifest.LanguageCounts,
		SliceCounts:          append([]SliceCount(nil), manifest.SliceCounts...),
		QuerySkeletonCount:   manifest.QuerySkeletonCount,
		AuditVerdict:         pool.Audit.Verdict,
		FixtureContentSHA256: manifest.FixtureContentSHA256,
		CorpusContentSHA256:  manifest.CorpusContentSHA256,
		AuditContentSHA256:   manifest.AuditContentSHA256,
		ManifestSHA256:       sha256Hex(pool.ManifestJSON),
	}
}
