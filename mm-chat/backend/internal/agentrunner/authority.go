package agentrunner

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const maxAuthorityTTL = 15 * time.Second

type AuthorityVerifier interface {
	Verify(caller, runnerID, method, requestID, nonce, requestFingerprint, snapshotFingerprint string,
		attempt AttemptRef, ticket AuthorityTicket, now time.Time) error
}

type SignedAuthorityVerifier struct{ publicKey ed25519.PublicKey }

func NewSignedAuthorityVerifier(publicKey ed25519.PublicKey) (*SignedAuthorityVerifier, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, ErrInvalidInput
	}
	copyKey := append(ed25519.PublicKey(nil), publicKey...)
	return &SignedAuthorityVerifier{publicKey: copyKey}, nil
}

func SignAuthority(privateKey ed25519.PrivateKey, claims AuthorityClaims) (AuthorityTicket, error) {
	if len(privateKey) != ed25519.PrivateKeySize || validateAuthorityClaims(claims) != nil {
		return AuthorityTicket{}, ErrInvalidInput
	}
	canonical, err := json.Marshal(claims)
	if err != nil {
		return AuthorityTicket{}, ErrInvalidInput
	}
	signature := ed25519.Sign(privateKey, append([]byte("neo-runner-authority-v1\x00"), canonical...))
	return AuthorityTicket{AuthorityClaims: claims,
		Signature: base64.RawURLEncoding.EncodeToString(signature)}, nil
}

func (verifier *SignedAuthorityVerifier) Verify(
	caller, runnerID, method, requestID, nonce, requestFingerprint, snapshotFingerprint string,
	attempt AttemptRef, ticket AuthorityTicket, now time.Time,
) error {
	if verifier == nil || len(verifier.publicKey) != ed25519.PublicKeySize ||
		validateAuthorityClaims(ticket.AuthorityClaims) != nil ||
		ticket.SchemaVersion != AuthorityVersion || ticket.CallerIdentity != caller ||
		ticket.RunnerID != runnerID || ticket.Method != method || ticket.RequestID != requestID ||
		ticket.Nonce != nonce || ticket.RequestFingerprint != requestFingerprint ||
		ticket.RunID != attempt.RunID || ticket.StepID != attempt.StepID ||
		ticket.AttemptID != attempt.AttemptID || ticket.LeaseGeneration != attempt.LeaseGeneration ||
		ticket.LeaseOwner != attempt.LeaseOwner || ticket.SnapshotFingerprint != snapshotFingerprint ||
		now.Before(ticket.IssuedAt.Add(-time.Second)) || !now.Before(ticket.ExpiresAt) ||
		ticket.ExpiresAt.Sub(ticket.IssuedAt) > maxAuthorityTTL {
		return ErrAuthFailed
	}
	digest := sha256.Sum256([]byte(attempt.LeaseToken))
	expectedDigest := hex.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(expectedDigest), []byte(ticket.LeaseTokenDigest)) != 1 {
		return ErrLeaseStale
	}
	signature, err := base64.RawURLEncoding.DecodeString(ticket.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ErrAuthFailed
	}
	canonical, _ := json.Marshal(ticket.AuthorityClaims)
	if !ed25519.Verify(verifier.publicKey,
		append([]byte("neo-runner-authority-v1\x00"), canonical...), signature) {
		return ErrAuthFailed
	}
	return nil
}

func NewAuthorityClaims(caller, runnerID, method, requestID, nonce, requestFingerprint, snapshotFingerprint string,
	attempt AttemptRef, killSwitchEpoch int64, now, expiresAt time.Time) AuthorityClaims {
	digest := sha256.Sum256([]byte(attempt.LeaseToken))
	return AuthorityClaims{
		SchemaVersion: AuthorityVersion, RunnerID: runnerID, CallerIdentity: caller,
		Method: method, RequestID: requestID, Nonce: nonce, RequestFingerprint: requestFingerprint,
		RunID: attempt.RunID, StepID: attempt.StepID,
		AttemptID: attempt.AttemptID, LeaseGeneration: attempt.LeaseGeneration,
		LeaseOwner: attempt.LeaseOwner, LeaseTokenDigest: hex.EncodeToString(digest[:]),
		SnapshotFingerprint: snapshotFingerprint, KillSwitchEpoch: killSwitchEpoch,
		IssuedAt: now.UTC(), ExpiresAt: expiresAt.UTC(),
	}
}

func validateAuthorityClaims(claims AuthorityClaims) error {
	if claims.SchemaVersion != AuthorityVersion || !identityPattern.MatchString(claims.RunnerID) ||
		!identityPattern.MatchString(claims.CallerIdentity) ||
		!member(claims.Method, MethodLaunch, MethodHeartbeat, MethodCancel, MethodPrepare, MethodCommit) ||
		!validID(claims.RequestID, "rpc") || !noncePattern.MatchString(claims.Nonce) ||
		!validFingerprint(claims.RequestFingerprint) || !validID(claims.RunID, "run") ||
		!validID(claims.StepID, "step") || !validID(claims.AttemptID, "attempt") ||
		claims.LeaseGeneration < 1 || !identityPattern.MatchString(claims.LeaseOwner) ||
		len(claims.LeaseTokenDigest) != 64 || !isLowerHex(claims.LeaseTokenDigest) ||
		!validFingerprint(claims.SnapshotFingerprint) || claims.KillSwitchEpoch < 0 ||
		claims.IssuedAt.IsZero() || !claims.ExpiresAt.After(claims.IssuedAt) ||
		claims.ExpiresAt.Sub(claims.IssuedAt) > maxAuthorityTTL {
		return ErrInvalidInput
	}
	return nil
}

func LoadEd25519PublicKey(path string) (ed25519.PublicKey, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("authority public key path is invalid")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 512 || info.Mode().Perm()&0o022 != 0 {
		return nil, errors.New("authority public key file is invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("authority public key is unavailable")
	}
	defer clear(data)
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("authority public key is invalid")
	}
	return ed25519.PublicKey(append([]byte(nil), decoded...)), nil
}

func LoadEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") || securePrivateFile(path) != nil {
		return nil, errors.New("authority private key file is invalid")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 512 {
		return nil, errors.New("authority private key is unavailable")
	}
	defer clear(data)
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		return nil, errors.New("authority private key is invalid")
	}
	return ed25519.PrivateKey(append([]byte(nil), decoded...)), nil
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func authorityError(err error) error {
	if errors.Is(err, ErrLeaseStale) {
		return ErrLeaseStale
	}
	return fmt.Errorf("%w", ErrAuthFailed)
}
