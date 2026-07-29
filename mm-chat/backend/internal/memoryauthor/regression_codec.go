package memoryauthor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

func DecodeRegressionFixtureManifest(reader io.Reader) (RegressionFixtureManifest, error) {
	body, err := readBounded(reader, maximumArtifactBytes)
	if err != nil {
		return RegressionFixtureManifest{}, fmt.Errorf("read regression fixture manifest: %w", err)
	}
	var manifest RegressionFixtureManifest
	if err := strictjson.Decode(body, maximumArtifactBytes, &manifest); err != nil {
		return RegressionFixtureManifest{}, fmt.Errorf("decode regression fixture manifest: %w", err)
	}
	if err := validateRegressionFixtureManifest(manifest); err != nil {
		return RegressionFixtureManifest{}, err
	}
	digest, err := RegressionFixtureContentSHA256(manifest)
	if err != nil {
		return RegressionFixtureManifest{}, err
	}
	if digest != manifest.ContentSHA256 {
		return RegressionFixtureManifest{}, errors.New("regression fixture content hash does not match")
	}
	return manifest, nil
}

func DecodeRegressionManifest(reader io.Reader) (RegressionManifest, error) {
	body, err := readBounded(reader, maximumArtifactBytes)
	if err != nil {
		return RegressionManifest{}, fmt.Errorf("read regression manifest: %w", err)
	}
	var manifest RegressionManifest
	if err := strictjson.Decode(body, maximumArtifactBytes, &manifest); err != nil {
		return RegressionManifest{}, fmt.Errorf("decode regression manifest: %w", err)
	}
	if err := validateRegressionManifest(manifest); err != nil {
		return RegressionManifest{}, err
	}
	return manifest, nil
}

func RegressionFixtureContentSHA256(manifest RegressionFixtureManifest) (string, error) {
	manifest.ContentSHA256 = ""
	body, err := json.Marshal(manifest)
	if err != nil {
		return "", errors.New("encode regression fixture manifest")
	}
	return sha256Hex(body), nil
}
