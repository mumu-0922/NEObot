package agentbroker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const maxPublishedArtifactBytes = int64(32 << 20)

type ArtifactCandidate struct {
	UserID        string
	RunID         string
	AttemptID     string
	Generation    int64
	QuarantineRef string
	Name          string
	MediaType     string
	Size          int64
	Fingerprint   string
}

type ArtifactAuthority struct {
	UserID          string
	RunID           string
	AttemptID       string
	Generation      int64
	MaxBytes        int64
	AllowedMedia    []string
	Current         bool
	GrantAuthorized bool
}

type ArtifactScanner interface {
	Scan(context.Context, ArtifactCandidate, io.Reader) error
}

type ArtifactQuarantine interface {
	Open(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}

type ArtifactObjectStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Delete(context.Context, string) error
}

type ArtifactPublicationRepository interface {
	AttachArtifact(context.Context, ArtifactCandidate, string) error
}

type ArtifactPublisher struct {
	quarantine ArtifactQuarantine
	objects    ArtifactObjectStore
	repository ArtifactPublicationRepository
	scanner    ArtifactScanner
}

func NewArtifactPublisher(quarantine ArtifactQuarantine, objects ArtifactObjectStore,
	repository ArtifactPublicationRepository, scanner ArtifactScanner) (*ArtifactPublisher, error) {
	if quarantine == nil || objects == nil || repository == nil || scanner == nil {
		return nil, ErrInvalidInput
	}
	return &ArtifactPublisher{quarantine: quarantine, objects: objects, repository: repository, scanner: scanner}, nil
}

func (publisher *ArtifactPublisher) Publish(ctx context.Context, authority ArtifactAuthority, candidate ArtifactCandidate) (string, error) {
	if publisher == nil || !authority.Current || !authority.GrantAuthorized || authority.UserID != candidate.UserID ||
		authority.RunID != candidate.RunID || authority.AttemptID != candidate.AttemptID ||
		authority.Generation != candidate.Generation || candidate.Size < 0 || candidate.Size > authority.MaxBytes ||
		candidate.Size > maxPublishedArtifactBytes ||
		!validFingerprint(candidate.Fingerprint) || !validArtifactRef(candidate) || !containsExact(authority.AllowedMedia, candidate.MediaType) {
		return "", ErrArtifactDenied
	}
	reader, err := publisher.quarantine.Open(ctx, candidate.QuarantineRef)
	if err != nil {
		return "", ErrArtifactDenied
	}
	payload, readErr := io.ReadAll(io.LimitReader(reader, candidate.Size+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || int64(len(payload)) != candidate.Size || artifactFingerprint(payload) != candidate.Fingerprint {
		_ = publisher.quarantine.Delete(context.WithoutCancel(ctx), candidate.QuarantineRef)
		return "", ErrArtifactDenied
	}
	defer clear(payload)
	if err := publisher.scanner.Scan(ctx, candidate, bytes.NewReader(payload)); err != nil {
		_ = publisher.quarantine.Delete(context.WithoutCancel(ctx), candidate.QuarantineRef)
		return "", ErrArtifactDenied
	}
	objectKey := fmt.Sprintf("agent-artifacts/%s/%s/%d/%s", candidate.RunID, candidate.AttemptID, candidate.Generation, candidate.Fingerprint[7:])
	if err := publisher.objects.Put(ctx, objectKey, bytes.NewReader(payload), candidate.Size, candidate.MediaType); err != nil {
		return "", ErrArtifactDenied
	}
	if err := publisher.repository.AttachArtifact(ctx, candidate, objectKey); err != nil {
		_ = publisher.objects.Delete(context.WithoutCancel(ctx), objectKey)
		return "", err
	}
	if err := publisher.quarantine.Delete(context.WithoutCancel(ctx), candidate.QuarantineRef); err != nil {
		// Publication is already authoritative. Cleanup remains retryable and may
		// not roll back the attached artifact row.
		return objectKey, nil
	}
	return objectKey, nil
}

func artifactFingerprint(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validArtifactRef(candidate ArtifactCandidate) bool {
	return candidate.Name != "" && len(candidate.Name) <= 128 && !strings.ContainsAny(candidate.Name, "/\\\x00\r\n") &&
		candidate.QuarantineRef == candidate.AttemptID+"/"+candidate.Name &&
		candidate.MediaType != "" && len(candidate.MediaType) <= 128 && !strings.ContainsAny(candidate.MediaType, "\x00\r\n")
}

func containsExact(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
