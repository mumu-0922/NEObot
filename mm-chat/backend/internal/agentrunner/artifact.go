package agentrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const maxResultArtifactBytes = int64(64 << 10)

const maxArtifactBytes = int64(32 << 20)

var artifactNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type ArtifactMetadata struct {
	Name      string
	MediaType string
	Size      int64
}

type ArtifactReceipt struct {
	Attempt       AttemptIdentity `json:"attempt"`
	Name          string          `json:"name"`
	MediaType     string          `json:"mediaType"`
	Size          int64           `json:"size"`
	Fingerprint   string          `json:"fingerprint"`
	QuarantineRef string          `json:"quarantineRef"`
}

type ArtifactBroker struct {
	root string
	mu   sync.Mutex
	used map[string]int64
	max  map[string]int64
}

func NewArtifactBroker(root string) (*ArtifactBroker, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if !filepath.IsAbs(root) || err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, ErrInvalidInput
	}
	return &ArtifactBroker{root: root, used: map[string]int64{}, max: map[string]int64{}}, nil
}

func (broker *ArtifactBroker) AuthorizeAttempt(attempt AttemptIdentity, maxBytes int64) error {
	if broker == nil || !validID(attempt.AttemptID, "attempt") || attempt.LeaseGeneration < 1 ||
		maxBytes < 1 || maxBytes > maxArtifactBytes {
		return ErrInvalidInput
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	used, err := broker.inspectAttempt(attempt.AttemptID, maxBytes)
	if err != nil {
		return err
	}
	broker.max[attemptKey(attempt)] = maxBytes
	broker.used[attemptKey(attempt)] = used
	return nil
}

func (broker *ArtifactBroker) inspectAttempt(attemptID string, maxBytes int64) (int64, error) {
	directory := filepath.Join(broker.root, attemptID)
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, ErrRuntimeUnavailable
	}
	var used int64
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		if strings.HasPrefix(entry.Name(), ".partial-") {
			if entry.Type().IsRegular() {
				if err := os.Remove(path); err != nil {
					return 0, ErrRuntimeUnavailable
				}
				continue
			}
			return 0, ErrRuntimeUnavailable
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
			!artifactNamePattern.MatchString(entry.Name()) || info.Size() < 0 || used+info.Size() > maxBytes {
			return 0, ErrRuntimeUnavailable
		}
		used += info.Size()
	}
	return used, nil
}

type ArtifactUpload struct {
	broker   *ArtifactBroker
	attempt  AttemptIdentity
	metadata ArtifactMetadata
	file     *os.File
	path     string
	digest   hashWriter
	written  int64
	closed   bool
	reserved bool
}

type hashWriter interface {
	io.Writer
	Sum([]byte) []byte
}

func (broker *ArtifactBroker) Begin(attempt AttemptIdentity, metadata ArtifactMetadata) (*ArtifactUpload, error) {
	if broker == nil || !validID(attempt.RunID, "run") || !validID(attempt.StepID, "step") ||
		!validID(attempt.AttemptID, "attempt") || attempt.LeaseGeneration < 1 ||
		!artifactNamePattern.MatchString(metadata.Name) || filepath.Base(metadata.Name) != metadata.Name ||
		!member(metadata.MediaType, "application/octet-stream", "application/json", "text/plain", "image/png", "image/jpeg") ||
		metadata.Size < 0 || metadata.Size > maxArtifactBytes {
		return nil, ErrInvalidInput
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	key := attemptKey(attempt)
	limit, authorized := broker.max[key]
	if !authorized || broker.used[key]+metadata.Size > limit {
		return nil, ErrLeaseStale
	}
	broker.used[key] += metadata.Size
	directory := filepath.Join(broker.root, attempt.AttemptID)
	if !pathWithin(broker.root, directory) {
		return nil, ErrInvalidInput
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, ErrRuntimeUnavailable
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, ErrRuntimeUnavailable
	}
	file, err := os.OpenFile(filepath.Join(directory, ".partial-"+metadata.Name),
		os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		broker.used[key] -= metadata.Size
		return nil, ErrRuntimeUnavailable
	}
	return &ArtifactUpload{broker: broker, attempt: attempt, metadata: metadata, file: file,
		path: file.Name(), digest: sha256.New(), reserved: true}, nil
}

func (upload *ArtifactUpload) Write(data []byte) (int, error) {
	if upload == nil || upload.closed {
		return 0, errors.New("artifact upload is closed")
	}
	remaining := upload.metadata.Size - upload.written
	if int64(len(data)) > remaining {
		return 0, ErrInvalidInput
	}
	n, err := io.MultiWriter(upload.file, upload.digest).Write(data)
	upload.written += int64(n)
	return n, err
}

func (upload *ArtifactUpload) Finalize(current AttemptIdentity) (ArtifactReceipt, error) {
	if upload == nil || upload.closed || current != upload.attempt || upload.written != upload.metadata.Size {
		if upload != nil {
			_ = upload.Abort()
		}
		if current != upload.attempt {
			return ArtifactReceipt{}, ErrLeaseStale
		}
		return ArtifactReceipt{}, ErrInvalidInput
	}
	upload.closed = true
	if err := upload.file.Sync(); err != nil {
		upload.file.Close()
		os.Remove(upload.path)
		upload.releaseReservation()
		return ArtifactReceipt{}, ErrRuntimeUnavailable
	}
	if err := upload.file.Close(); err != nil {
		os.Remove(upload.path)
		upload.releaseReservation()
		return ArtifactReceipt{}, ErrRuntimeUnavailable
	}
	fingerprint := "sha256:" + hex.EncodeToString(upload.digest.Sum(nil))
	finalPath := filepath.Join(filepath.Dir(upload.path), strings.TrimPrefix(filepath.Base(upload.path), ".partial-"))
	if err := os.Rename(upload.path, finalPath); err != nil {
		os.Remove(upload.path)
		upload.releaseReservation()
		return ArtifactReceipt{}, ErrRuntimeUnavailable
	}
	if err := syncDirectory(filepath.Dir(finalPath)); err != nil {
		upload.releaseReservation()
		return ArtifactReceipt{}, ErrRuntimeUnavailable
	}
	upload.reserved = false
	return ArtifactReceipt{Attempt: upload.attempt, Name: upload.metadata.Name, MediaType: upload.metadata.MediaType,
		Size: upload.written, Fingerprint: fingerprint,
		QuarantineRef: upload.attempt.AttemptID + "/" + upload.metadata.Name}, nil
}

func (upload *ArtifactUpload) Abort() error {
	if upload == nil || upload.closed {
		return nil
	}
	upload.closed = true
	closeErr := upload.file.Close()
	removeErr := os.Remove(upload.path)
	upload.releaseReservation()
	if closeErr != nil {
		return closeErr
	}
	if removeErr != nil && !os.IsNotExist(removeErr) {
		return removeErr
	}
	return nil
}

func (upload *ArtifactUpload) releaseReservation() {
	if upload == nil || !upload.reserved {
		return
	}
	upload.broker.mu.Lock()
	key := attemptKey(upload.attempt)
	upload.broker.used[key] -= upload.metadata.Size
	if upload.broker.used[key] < 0 {
		upload.broker.used[key] = 0
	}
	upload.broker.mu.Unlock()
	upload.reserved = false
}

func (broker *ArtifactBroker) CleanupAttempt(attemptID string) error {
	if broker == nil || !validID(attemptID, "attempt") {
		return ErrInvalidInput
	}
	broker.mu.Lock()
	for key := range broker.max {
		if strings.HasPrefix(key, attemptID+"/") {
			delete(broker.max, key)
			delete(broker.used, key)
		}
	}
	broker.mu.Unlock()
	return safeRemoveTree(broker.root, filepath.Join(broker.root, attemptID))
}

// ReadResult returns one exact bounded artifact only while the Attempt
// generation remains authorized. The caller still owns domain validation.
func (broker *ArtifactBroker) ReadResult(ctx context.Context, attempt AttemptIdentity, name string) (ArtifactReceipt, []byte, error) {
	if broker == nil || ctx == nil || !validID(attempt.RunID, "run") || !validID(attempt.StepID, "step") ||
		!validID(attempt.AttemptID, "attempt") || attempt.LeaseGeneration < 1 ||
		!artifactNamePattern.MatchString(name) || filepath.Base(name) != name {
		return ArtifactReceipt{}, nil, ErrInvalidInput
	}
	broker.mu.Lock()
	defer broker.mu.Unlock()
	if _, authorized := broker.max[attemptKey(attempt)]; !authorized {
		return ArtifactReceipt{}, nil, ErrLeaseStale
	}
	path := filepath.Join(broker.root, attempt.AttemptID, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ArtifactReceipt{}, nil, ErrNotFound
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
		info.Size() < 1 || info.Size() > maxResultArtifactBytes {
		return ArtifactReceipt{}, nil, ErrArtifactDenied
	}
	select {
	case <-ctx.Done():
		return ArtifactReceipt{}, nil, ctx.Err()
	default:
	}
	body, err := os.ReadFile(path)
	if err != nil || int64(len(body)) != info.Size() {
		return ArtifactReceipt{}, nil, ErrRuntimeUnavailable
	}
	digest := sha256.Sum256(body)
	receipt := ArtifactReceipt{Attempt: attempt, Name: name, MediaType: "application/json",
		Size: int64(len(body)), Fingerprint: "sha256:" + hex.EncodeToString(digest[:]),
		QuarantineRef: attempt.AttemptID + "/" + name}
	return receipt, body, nil
}

func attemptKey(attempt AttemptIdentity) string {
	return attempt.AttemptID + "/" + strconv.FormatInt(attempt.LeaseGeneration, 10)
}
