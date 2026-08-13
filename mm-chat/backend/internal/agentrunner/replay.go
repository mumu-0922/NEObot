package agentrunner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"

	"golang.org/x/sys/unix"
)

const (
	maxReplayLedgerBytes = 32 << 20
	maxReplayRecords     = 100_000
)

type ReplayClaim struct {
	CallerIdentity     string
	RequestID          string
	Nonce              string
	Method             string
	RequestFingerprint string
	ExpiresAt          time.Time
}

type ReplayResult struct {
	Replay   bool
	Response []byte
}

type ReplayLedger interface {
	Claim(context.Context, ReplayClaim) (ReplayResult, error)
	Complete(context.Context, string, string, string, []byte) error
	Compact(context.Context) (int, error)
}

type replayRecord struct {
	Version            string    `json:"version"`
	Operation          string    `json:"operation"`
	CallerIdentity     string    `json:"callerIdentity"`
	RequestID          string    `json:"requestId"`
	Nonce              string    `json:"nonce"`
	Method             string    `json:"method"`
	RequestFingerprint string    `json:"requestFingerprint"`
	Response           []byte    `json:"response,omitempty"`
	ExpiresAt          time.Time `json:"expiresAt"`
	RecordedAt         time.Time `json:"recordedAt"`
}

type localReplayEntry struct {
	replayRecord
	Complete bool
}

type FileReplayLedger struct {
	path string
	now  func() time.Time
	mu   sync.Mutex
}

func NewFileReplayLedger(path string) (*FileReplayLedger, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || filepath.Base(path) == "." || filepath.Base(path) == string(filepath.Separator) {
		return nil, ErrInvalidInput
	}
	return &FileReplayLedger{path: path, now: time.Now}, nil
}

func (ledger *FileReplayLedger) Claim(_ context.Context, claim ReplayClaim) (ReplayResult, error) {
	if ledger == nil || validateReplayClaim(claim) != nil {
		return ReplayResult{}, ErrInvalidInput
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	file, err := ledger.openLocked()
	if err != nil {
		return ReplayResult{}, err
	}
	defer file.Close()
	entries, err := readReplayEntries(file)
	if err != nil {
		return ReplayResult{}, err
	}
	key := replayKey(claim.CallerIdentity, claim.RequestID, claim.Nonce)
	if existing, ok := entries[key]; ok {
		if existing.Method != claim.Method || existing.RequestFingerprint != claim.RequestFingerprint {
			return ReplayResult{}, ErrReplayDetected
		}
		if !existing.Complete {
			return ReplayResult{}, ErrReplayDetected
		}
		return ReplayResult{Replay: true, Response: append([]byte(nil), existing.Response...)}, nil
	}
	if len(entries) >= maxReplayRecords {
		return ReplayResult{}, ErrRuntimeUnavailable
	}
	record := replayRecord{Version: "neo.runner-local-replay/v1", Operation: "claim",
		CallerIdentity: claim.CallerIdentity, RequestID: claim.RequestID, Nonce: claim.Nonce,
		Method: claim.Method, RequestFingerprint: claim.RequestFingerprint,
		ExpiresAt: claim.ExpiresAt.UTC(), RecordedAt: ledger.now().UTC()}
	if err := appendReplayRecord(file, record); err != nil {
		return ReplayResult{}, err
	}
	return ReplayResult{}, nil
}

func (ledger *FileReplayLedger) Complete(_ context.Context, caller, requestID, nonce string, response []byte) error {
	if ledger == nil || !identityPattern.MatchString(caller) || !validID(requestID, "rpc") ||
		!noncePattern.MatchString(nonce) || !validReplayResponse(response) {
		return ErrInvalidInput
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	file, err := ledger.openLocked()
	if err != nil {
		return err
	}
	defer file.Close()
	entries, err := readReplayEntries(file)
	if err != nil {
		return err
	}
	key := replayKey(caller, requestID, nonce)
	entry, ok := entries[key]
	if !ok {
		return ErrNotFound
	}
	if entry.Complete {
		if bytes.Equal(entry.Response, response) {
			return nil
		}
		return ErrReplayDetected
	}
	record := entry.replayRecord
	record.Operation = "complete"
	record.Response = append([]byte(nil), response...)
	record.RecordedAt = ledger.now().UTC()
	return appendReplayRecord(file, record)
}

func (ledger *FileReplayLedger) Compact(ctx context.Context) (int, error) {
	if ledger == nil {
		return 0, ErrInvalidInput
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	file, err := ledger.openLocked()
	if err != nil {
		return 0, err
	}
	defer file.Close()
	entries, err := readReplayEntries(file)
	if err != nil {
		return 0, err
	}
	now := ledger.now()
	retained := make([]localReplayEntry, 0, len(entries))
	deleted := 0
	for _, entry := range entries {
		if entry.Complete && !entry.ExpiresAt.After(now) {
			deleted++
			continue
		}
		retained = append(retained, entry)
	}
	sort.Slice(retained, func(i, j int) bool {
		return replayKey(retained[i].CallerIdentity, retained[i].RequestID, retained[i].Nonce) <
			replayKey(retained[j].CallerIdentity, retained[j].RequestID, retained[j].Nonce)
	})
	temporary := ledger.path + ".compact"
	temp, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return 0, ErrRuntimeUnavailable
	}
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(temporary)
		}
	}()
	for _, entry := range retained {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		claim := entry.replayRecord
		claim.Operation = "claim"
		claim.Response = nil
		if err := appendReplayRecord(temp, claim); err != nil {
			return 0, err
		}
		if entry.Complete {
			complete := entry.replayRecord
			complete.Operation = "complete"
			if err := appendReplayRecord(temp, complete); err != nil {
				return 0, err
			}
		}
	}
	if err := temp.Close(); err != nil {
		return 0, err
	}
	if err := os.Rename(temporary, ledger.path); err != nil {
		return 0, err
	}
	cleanup = false
	if err := syncDirectory(filepath.Dir(ledger.path)); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (ledger *FileReplayLedger) openLocked() (*os.File, error) {
	directory := filepath.Dir(ledger.path)
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("Runner replay directory is unavailable")
	}
	file, err := os.OpenFile(ledger.path, os.O_RDWR|os.O_CREATE|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, errors.New("Runner replay ledger is unavailable")
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() > maxReplayLedgerBytes {
		file.Close()
		return nil, errors.New("Runner replay ledger is invalid")
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		file.Close()
		return nil, errors.New("Runner replay ledger lock failed")
	}
	return file, nil
}

func readReplayEntries(file *os.File) (map[string]localReplayEntry, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	entries := map[string]localReplayEntry{}
	scanner := bufio.NewScanner(io.LimitReader(file, maxReplayLedgerBytes+1))
	scanner.Buffer(make([]byte, 64<<10), 512<<10)
	count := 0
	for scanner.Scan() {
		count++
		if count > maxReplayRecords {
			return nil, errors.New("Runner replay ledger is oversized")
		}
		line := append([]byte(nil), scanner.Bytes()...)
		var record replayRecord
		if err := strictjson.Decode(line, 512<<10, &record); err != nil || validateReplayRecord(record) != nil {
			return nil, errors.New("Runner replay ledger is corrupt")
		}
		key := replayKey(record.CallerIdentity, record.RequestID, record.Nonce)
		existing, exists := entries[key]
		if !exists {
			if record.Operation != "claim" {
				return nil, errors.New("Runner replay ledger is corrupt")
			}
			entries[key] = localReplayEntry{replayRecord: record}
			continue
		}
		if record.Operation != "complete" || existing.Complete ||
			existing.Method != record.Method || existing.RequestFingerprint != record.RequestFingerprint ||
			!existing.ExpiresAt.Equal(record.ExpiresAt) {
			return nil, errors.New("Runner replay ledger is corrupt")
		}
		entries[key] = localReplayEntry{replayRecord: record, Complete: true}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func appendReplayRecord(file *os.File, record replayRecord) error {
	line, err := json.Marshal(record)
	if err != nil || len(line) > 512<<10 {
		return ErrInvalidInput
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return nil
}

func validateReplayClaim(claim ReplayClaim) error {
	if !identityPattern.MatchString(claim.CallerIdentity) || !validID(claim.RequestID, "rpc") ||
		!noncePattern.MatchString(claim.Nonce) || !member(claim.Method, MethodProbe, MethodLaunch,
		MethodHeartbeat, MethodCancel, MethodPrepare, MethodCommit, MethodList, MethodReconcile) || !validFingerprint(claim.RequestFingerprint) ||
		claim.ExpiresAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

func validateReplayRecord(record replayRecord) error {
	if record.Version != "neo.runner-local-replay/v1" ||
		!member(record.Operation, "claim", "complete") || validateReplayClaim(ReplayClaim{
		CallerIdentity: record.CallerIdentity, RequestID: record.RequestID, Nonce: record.Nonce,
		Method: record.Method, RequestFingerprint: record.RequestFingerprint, ExpiresAt: record.ExpiresAt,
	}) != nil || record.RecordedAt.IsZero() {
		return ErrInvalidInput
	}
	if record.Operation == "claim" && len(record.Response) != 0 {
		return ErrInvalidInput
	}
	if record.Operation == "complete" && !validReplayResponse(record.Response) {
		return ErrInvalidInput
	}
	return nil
}

func validReplayResponse(body []byte) bool {
	if len(body) < 2 || len(body) > 256<<10 {
		return false
	}
	var value any
	if strictjson.Decode(body, 256<<10, &value) != nil {
		return false
	}
	root, ok := value.(map[string]any)
	if !ok || len(root) < 5 {
		return false
	}
	return replayValueSanitized(value)
}

func replayValueSanitized(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			if member(normalized, "leasetoken", "stdout", "stderr", "content", "workspacecontent",
				"skillbody", "toolarguments", "toolresults", "secret", "secretvalue",
				"authorization", "apikey", "password", "accesstoken", "refreshtoken",
				"credential", "hostpath", "sourcepath", "certificate", "privatekey") ||
				strings.Contains(normalized, "password") || strings.Contains(normalized, "authorization") ||
				strings.Contains(normalized, "privatekey") || !replayValueSanitized(child) {
				return false
			}
		}
	case []any:
		for _, child := range typed {
			if !replayValueSanitized(child) {
				return false
			}
		}
	}
	return true
}

func replayKey(caller, requestID, nonce string) string {
	digest := sha256.Sum256([]byte(caller + "\x00" + requestID + "\x00" + nonce))
	return hex.EncodeToString(digest[:])
}

type MemoryReplayLedger struct {
	mu      sync.Mutex
	entries map[string]localReplayEntry
}

func NewMemoryReplayLedger() *MemoryReplayLedger {
	return &MemoryReplayLedger{entries: map[string]localReplayEntry{}}
}

func (ledger *MemoryReplayLedger) Claim(_ context.Context, claim ReplayClaim) (ReplayResult, error) {
	if validateReplayClaim(claim) != nil {
		return ReplayResult{}, ErrInvalidInput
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	key := replayKey(claim.CallerIdentity, claim.RequestID, claim.Nonce)
	if entry, ok := ledger.entries[key]; ok {
		if entry.Method != claim.Method || entry.RequestFingerprint != claim.RequestFingerprint || !entry.Complete {
			return ReplayResult{}, ErrReplayDetected
		}
		return ReplayResult{Replay: true, Response: append([]byte(nil), entry.Response...)}, nil
	}
	ledger.entries[key] = localReplayEntry{replayRecord: replayRecord{Method: claim.Method,
		CallerIdentity: claim.CallerIdentity, RequestID: claim.RequestID, Nonce: claim.Nonce,
		RequestFingerprint: claim.RequestFingerprint, ExpiresAt: claim.ExpiresAt}}
	return ReplayResult{}, nil
}

func (ledger *MemoryReplayLedger) Complete(_ context.Context, caller, requestID, nonce string, response []byte) error {
	if !validReplayResponse(response) {
		return ErrInvalidInput
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	key := replayKey(caller, requestID, nonce)
	entry, ok := ledger.entries[key]
	if !ok {
		return ErrNotFound
	}
	if entry.Complete && !bytes.Equal(entry.Response, response) {
		return ErrReplayDetected
	}
	entry.Complete = true
	entry.Response = append([]byte(nil), response...)
	ledger.entries[key] = entry
	return nil
}

func (ledger *MemoryReplayLedger) Compact(_ context.Context) (int, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	deleted := 0
	for key, entry := range ledger.entries {
		if entry.Complete && !entry.ExpiresAt.After(time.Now()) {
			delete(ledger.entries, key)
			deleted++
		}
	}
	return deleted, nil
}

func replayError(operation string, err error) error {
	return fmt.Errorf("%s Runner replay: %w", operation, err)
}
