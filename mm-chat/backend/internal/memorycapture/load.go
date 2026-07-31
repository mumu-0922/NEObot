package memorycapture

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

// ProtectedRegression is one byte-verified, known-version machine-reviewed
// pool plus the raw hashes used by capture configuration provenance.
type ProtectedRegression struct {
	Pool              memoryauthor.RegressionPool
	FixtureRawSHA256  string
	CorpusRawSHA256   string
	AuditRawSHA256    string
	ManifestRawSHA256 string
}

// LoadProtectedRegression requires exact generator replay in addition to
// strict schema/hash admission. Unknown generators and mixed-version artifacts
// are never accepted by the native capture lane.
func LoadProtectedRegression(root string) (ProtectedRegression, error) {
	if _, err := memoryauthor.VerifyRegression(root); err != nil {
		return ProtectedRegression{}, fmt.Errorf("%w: verify protected regression: %v", ErrCaptureInvalid, err)
	}
	pool, err := memoryauthor.LoadRegression(root)
	if err != nil {
		return ProtectedRegression{}, fmt.Errorf("%w: load protected regression: %v", ErrCaptureInvalid, err)
	}
	result := ProtectedRegression{Pool: pool}
	for name, target := range map[string]*string{
		memoryauthor.RegressionFixtureFile:  &result.FixtureRawSHA256,
		memoryauthor.RegressionCorpusFile:   &result.CorpusRawSHA256,
		memoryauthor.RegressionAuditFile:    &result.AuditRawSHA256,
		memoryauthor.RegressionManifestFile: &result.ManifestRawSHA256,
	} {
		body, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			return ProtectedRegression{}, fmt.Errorf("%w: hash protected regression artifact", ErrCaptureInvalid)
		}
		digest := sha256.Sum256(body)
		*target = hex.EncodeToString(digest[:])
	}
	if result.FixtureRawSHA256 != pool.Manifest.FixtureRawSHA256 ||
		result.CorpusRawSHA256 != pool.Manifest.CorpusRawSHA256 ||
		result.AuditRawSHA256 != pool.Manifest.AuditRawSHA256 {
		return ProtectedRegression{}, fmt.Errorf("%w: protected regression raw hash drift", ErrCaptureInvalid)
	}
	manifestDigest := sha256.Sum256(pool.ManifestJSON)
	if result.ManifestRawSHA256 != hex.EncodeToString(manifestDigest[:]) {
		return ProtectedRegression{}, errors.New("protected regression manifest hash drift")
	}
	return result, nil
}
