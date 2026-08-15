package agentrunner

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type IssueAuthorityInput struct {
	CallerIdentity      string
	RunnerID            string
	RequestID           string
	Nonce               string
	Method              string
	RequestFingerprint  string
	UserID              string
	Attempt             AttemptRef
	SnapshotFingerprint string
	TTL                 time.Duration
}

type AuthorityRecord struct {
	AuthorityClaims
	Replay   bool
	Response json.RawMessage
}

type ExpectedSandbox struct {
	SandboxID           string
	CallerIdentity      string
	RequestID           string
	Nonce               string
	UserID              string
	Attempt             AttemptIdentity
	RunnerID            string
	SnapshotFingerprint string
	SpecFingerprint     string
	ProbeFingerprint    string
}

type RecoverySandbox struct {
	ExpectedSandbox
	SandboxState   string
	AttemptState   string
	LeaseExpiresAt time.Time
	LeaseExpired   bool
}

type ControlRepository interface {
	IssueAuthority(context.Context, IssueAuthorityInput, string) (AuthorityRecord, error)
	CompleteRequest(context.Context, string, string, string, string, string, []byte) error
	ExpectSandbox(context.Context, ExpectedSandbox) error
	UpdateSandbox(context.Context, string, string, int64, string, string, string) error
	RecoverySandboxes(context.Context, int) ([]RecoverySandbox, error)
	Prune(context.Context, time.Time, int) (int, int, error)
}

type ControlService struct {
	repository ControlRepository
	privateKey ed25519.PrivateKey
}

func NewControlService(repository ControlRepository, privateKey ed25519.PrivateKey) (*ControlService, error) {
	if repository == nil || len(privateKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidInput
	}
	return &ControlService{repository: repository, privateKey: append(ed25519.PrivateKey(nil), privateKey...)}, nil
}

func (service *ControlService) IssueAuthority(ctx context.Context, input IssueAuthorityInput) (AuthorityTicket, json.RawMessage, error) {
	if service == nil || validateIssueAuthority(input) != nil {
		return AuthorityTicket{}, nil, ErrInvalidInput
	}
	digest := sha256.Sum256([]byte(input.Attempt.LeaseToken))
	record, err := service.repository.IssueAuthority(ctx, input, hex.EncodeToString(digest[:]))
	if err != nil {
		return AuthorityTicket{}, nil, err
	}
	if record.Replay {
		return AuthorityTicket{}, append(json.RawMessage(nil), record.Response...), nil
	}
	ticket, err := SignAuthority(service.privateKey, record.AuthorityClaims)
	if err != nil {
		return AuthorityTicket{}, nil, err
	}
	return ticket, nil, nil
}

func (service *ControlService) CompleteRequest(ctx context.Context, caller, requestID, nonce, requestFingerprint string, response []byte) error {
	if service == nil || !identityPattern.MatchString(caller) || !validID(requestID, "rpc") || !noncePattern.MatchString(nonce) || !validFingerprint(requestFingerprint) || !validReplayResponse(response) {
		return ErrInvalidInput
	}
	digest := sha256.Sum256(append([]byte("neo-runner-rpc-response-v1\x00"), response...))
	return service.repository.CompleteRequest(ctx, caller, requestID, nonce, requestFingerprint, "sha256:"+hex.EncodeToString(digest[:]), response)
}

func validateIssueAuthority(input IssueAuthorityInput) error {
	if !identityPattern.MatchString(input.CallerIdentity) || !identityPattern.MatchString(input.RunnerID) ||
		input.RunnerID != input.Attempt.LeaseOwner ||
		!validID(input.RequestID, "rpc") || !noncePattern.MatchString(input.Nonce) ||
		!member(input.Method, MethodLaunch, MethodHeartbeat, MethodCancel, MethodResult, MethodPrepare, MethodCommit) || !validFingerprint(input.RequestFingerprint) ||
		!uuidPattern.MatchString(input.UserID) || !validAttempt(input.Attempt) || !validFingerprint(input.SnapshotFingerprint) ||
		input.TTL < time.Second || input.TTL > maxAuthorityTTL {
		return ErrInvalidInput
	}
	return nil
}

func mapControlError(err error) error {
	if err == nil {
		return nil
	}
	value := err.Error()
	for _, candidate := range []error{ErrReplayDetected, ErrLeaseStale, ErrSnapshotMismatch, ErrKillSwitchActive, ErrInvalidTransition} {
		if errors.Is(err, candidate) || strings.Contains(value, candidate.Error()) {
			return candidate
		}
	}
	return err
}
