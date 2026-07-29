package memoryauthor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maximumArtifactBytes = 64 << 20

func DecodeFixtureManifest(reader io.Reader) (FixtureManifest, error) {
	manifest, err := decodeFixtureManifestForHash(reader)
	if err != nil {
		return FixtureManifest{}, err
	}
	digest, err := FixtureContentSHA256(manifest)
	if err != nil {
		return FixtureManifest{}, err
	}
	if manifest.ContentSHA256 != digest {
		return FixtureManifest{}, errors.New("fixture manifest content hash does not match")
	}
	return manifest, nil
}

func decodeFixtureManifestForHash(reader io.Reader) (FixtureManifest, error) {
	body, err := readBounded(reader, maximumArtifactBytes)
	if err != nil {
		return FixtureManifest{}, fmt.Errorf("read fixture manifest: %w", err)
	}
	var manifest FixtureManifest
	if err := strictjson.Decode(body, maximumArtifactBytes, &manifest); err != nil {
		return FixtureManifest{}, fmt.Errorf("decode fixture manifest: %w", err)
	}
	if err := validateFixtureManifest(manifest); err != nil {
		return FixtureManifest{}, err
	}
	return manifest, nil
}

func DecodeCandidateManifest(reader io.Reader) (CandidateManifest, error) {
	body, err := readBounded(reader, maximumArtifactBytes)
	if err != nil {
		return CandidateManifest{}, fmt.Errorf("read candidate manifest: %w", err)
	}
	var manifest CandidateManifest
	if err := strictjson.Decode(body, maximumArtifactBytes, &manifest); err != nil {
		return CandidateManifest{}, fmt.Errorf("decode candidate manifest: %w", err)
	}
	if err := validateCandidateManifest(manifest); err != nil {
		return CandidateManifest{}, err
	}
	return manifest, nil
}

func decodeReviewEvent(body []byte) (ReviewEvent, error) {
	var event ReviewEvent
	if err := strictjson.Decode(body, maximumArtifactBytes, &event); err != nil {
		return ReviewEvent{}, fmt.Errorf("decode review event: %w", err)
	}
	if err := validateReviewEvent(event); err != nil {
		return ReviewEvent{}, err
	}
	return event, nil
}

func decodeCheckpoint(body []byte) (ReviewCheckpoint, error) {
	var checkpoint ReviewCheckpoint
	if err := strictjson.Decode(body, maximumArtifactBytes, &checkpoint); err != nil {
		return ReviewCheckpoint{}, fmt.Errorf("decode review checkpoint: %w", err)
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return ReviewCheckpoint{}, err
	}
	return checkpoint, nil
}

func decodeFreezeManifest(body []byte) (FreezeManifest, error) {
	var manifest FreezeManifest
	if err := strictjson.Decode(body, maximumArtifactBytes, &manifest); err != nil {
		return FreezeManifest{}, fmt.Errorf("decode freeze manifest: %w", err)
	}
	if err := validateFreezeManifest(manifest); err != nil {
		return FreezeManifest{}, err
	}
	return manifest, nil
}

func FixtureContentSHA256(manifest FixtureManifest) (string, error) {
	manifest.ContentSHA256 = ""
	body, err := json.Marshal(manifest)
	if err != nil {
		return "", errors.New("encode fixture manifest")
	}
	return sha256Hex(body), nil
}

func CaseContentSHA256(snapshot CaseSnapshot) (string, error) {
	snapshot.Case.Review = memoryeval.Review{State: "draft"}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return "", errors.New("encode candidate case content")
	}
	return sha256Hex(body), nil
}

func fixtureSHA256(fixture Fixture) (string, error) {
	body, err := json.Marshal(fixture)
	if err != nil {
		return "", errors.New("encode fixture")
	}
	return sha256Hex(body), nil
}

func marshalCanonical(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("encode JSON artifact")
	}
	return append(body, '\n'), nil
}

func marshalPretty(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, errors.New("encode JSON artifact")
	}
	return append(body, '\n'), nil
}

func readBounded(reader io.Reader, maximum int) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("input is required")
	}
	body, err := io.ReadAll(io.LimitReader(reader, int64(maximum)+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maximum {
		return nil, errors.New("input size is invalid")
	}
	return body, nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("artifact must be a regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("artifact changed while opening")
	}
	return readBounded(file, maximumArtifactBytes)
}

func sha256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
