package agentprojectcanary

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	ApprovalSchemaVersion = "neo.agent-project-mutation-approval/v1"
	ApprovalStage         = "project_mutation_canary"
	maximumApprovalBytes  = 64 << 10
	maximumApprovalWindow = 15 * time.Minute
)

var (
	ErrInvalidApproval = errors.New("PROJECT_CANARY_APPROVAL_INVALID")
	approvalIDPattern  = regexp.MustCompile(`^approval_[A-Za-z0-9_-]{16,128}$`)
	actorIDPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@/-]{0,127}$`)
	reasonCodePattern  = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	commitPattern      = regexp.MustCompile(`^[a-f0-9]{40}$`)
)

type ApprovalRelease struct {
	GitCommit     string `json:"gitCommit"`
	MigrationHead int    `json:"migrationHead"`
}

type ApprovalTarget struct {
	DeploymentFingerprint string `json:"deploymentFingerprint"`
	RunnerID              string `json:"runnerId"`
}

type ApprovalActivation struct {
	Stage                 string `json:"stage"`
	ActivationFingerprint string `json:"activationFingerprint"`
	PlanFingerprint       string `json:"planFingerprint"`
}

type ApprovalRequest struct {
	CallerIdentity  string `json:"callerIdentity"`
	RequestIdentity string `json:"requestIdentity"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type ApprovalAction struct {
	ToolIdentity        string `json:"toolIdentity"`
	Capability          string `json:"capability"`
	Action              string `json:"action"`
	Resource            string `json:"resource"`
	BaseRevision        string `json:"baseRevision"`
	Path                string `json:"path"`
	ContentFingerprint  string `json:"contentFingerprint"`
	MutationFingerprint string `json:"mutationFingerprint"`
}

type ApprovalActor struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	ReasonCode string `json:"reasonCode"`
}

type ApprovalWindow struct {
	IssuedAt  time.Time `json:"issuedAt"`
	NotBefore time.Time `json:"notBefore"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type ApprovalPayload struct {
	SchemaVersion string             `json:"schemaVersion"`
	ApprovalID    string             `json:"approvalId"`
	Decision      string             `json:"decision"`
	Release       ApprovalRelease    `json:"release"`
	Target        ApprovalTarget     `json:"target"`
	Activation    ApprovalActivation `json:"activation"`
	Request       ApprovalRequest    `json:"request"`
	Action        ApprovalAction     `json:"action"`
	Actor         ApprovalActor      `json:"actor"`
	Window        ApprovalWindow     `json:"window"`
}

type signedApprovalDocument struct {
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

type ApprovalBinding struct {
	ReleaseCommit         string
	TargetFingerprint     string
	RunnerID              string
	ActivationFingerprint string
	PlanFingerprint       string
	CallerIdentity        string
	RequestIdentity       string
	IdempotencyKey        string
	Action                Binding
}

type Approval struct {
	Payload              ApprovalPayload
	DocumentFingerprint  string
	PublicKeyFingerprint string
	signedPayload        []byte
}

// ActivationBindingFingerprint deliberately excludes the activation record's
// raw-file fingerprint. The record binds the signed approval document, so
// putting the record hash back into that document would create an impossible
// hash cycle. This domain-separated identity still freezes the exact release,
// target, Runner, plan, caller and private relay selected for the activation.
func ActivationBindingFingerprint(releaseCommit, targetFingerprint, runnerID, planFingerprint,
	callerIdentity, relayEndpoint string,
) string {
	return domainFingerprint("neo-agent-project-canary-activation-binding-v1", []byte(strings.Join([]string{
		ApprovalStage, releaseCommit, targetFingerprint, runnerID, planFingerprint, callerIdentity, relayEndpoint,
	}, "\n")))
}

func LoadApproval(documentPath, publicKeyPath string, binding ApprovalBinding, now time.Time) (*Approval, error) {
	documentRaw, err := readApprovalFile(documentPath, true)
	if err != nil {
		return nil, ErrInvalidApproval
	}
	publicKeyRaw, err := readApprovalFile(publicKeyPath, false)
	if err != nil {
		return nil, ErrInvalidApproval
	}
	defer clear(publicKeyRaw)
	decodedKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(publicKeyRaw)))
	if err != nil || len(decodedKey) != ed25519.PublicKeySize {
		clear(decodedKey)
		return nil, ErrInvalidApproval
	}
	publicKey := ed25519.PublicKey(decodedKey)
	defer clear(publicKey)
	var document signedApprovalDocument
	if strictjson.Decode(documentRaw, maximumApprovalBytes, &document) != nil || len(document.Payload) == 0 {
		return nil, ErrInvalidApproval
	}
	signature, err := base64.RawURLEncoding.DecodeString(document.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		clear(signature)
		return nil, ErrInvalidApproval
	}
	defer clear(signature)
	signingInput := append([]byte("neo-agent-project-mutation-approval-v1\x00"), document.Payload...)
	defer clear(signingInput)
	if !ed25519.Verify(publicKey, signingInput, signature) {
		return nil, ErrInvalidApproval
	}
	var payload ApprovalPayload
	if strictjson.Decode(document.Payload, maximumApprovalBytes, &payload) != nil ||
		validateApprovalPayload(payload, binding, now.UTC()) != nil {
		return nil, ErrInvalidApproval
	}
	return &Approval{Payload: payload,
		DocumentFingerprint:  rawFingerprint(documentRaw),
		PublicKeyFingerprint: rawFingerprint(publicKeyRaw),
		signedPayload:        append([]byte(nil), document.Payload...)}, nil
}

func (approval *Approval) VerifyPrepared(prepared agentbroker.PreparedIntent, binding Binding, now time.Time) error {
	if approval == nil || validateApprovalAction(approval.Payload.Action, binding) != nil ||
		approval.Payload.Request.RequestIdentity != binding.Plan.RequestIdentity ||
		approval.Payload.Request.IdempotencyKey != binding.Plan.IdempotencyKey ||
		prepared.Subject.UserID != binding.Grant.Subject.UserID || prepared.RunID != binding.RunID ||
		prepared.StepID != binding.StepID || prepared.GrantID != binding.Grant.GrantID ||
		prepared.GrantFingerprint != binding.GrantFingerprint() ||
		prepared.RegistryFingerprint != binding.Registry.Fingerprint ||
		prepared.SnapshotFingerprint != binding.SnapshotFingerprint ||
		prepared.ToolIdentity != binding.Plan.ToolIdentity || prepared.Capability != binding.Plan.Capability ||
		prepared.Action != binding.Plan.Action || prepared.Resource != binding.Plan.Resource ||
		prepared.ArgumentsFingerprint != binding.ArgumentsFingerprint ||
		prepared.BaseRevision != binding.Plan.BaseRevision || prepared.ApprovalClass != agentbroker.ApprovalPerCommit ||
		prepared.State != agentbroker.IntentAwaitingApproval || !now.UTC().Before(prepared.ExpiresAt) ||
		now.UTC().Before(approval.Payload.Window.NotBefore) || !now.UTC().Before(approval.Payload.Window.ExpiresAt) {
		return ErrInvalidApproval
	}
	return nil
}

func (approval *Approval) VerifyPrepareResult(prepared agentrunner.PrepareResult, binding Binding, now time.Time) error {
	if approval == nil || validateApprovalAction(approval.Payload.Action, binding) != nil ||
		approval.Payload.Request.RequestIdentity != binding.Plan.RequestIdentity ||
		approval.Payload.Request.IdempotencyKey != binding.Plan.IdempotencyKey ||
		!prepared.Prepared || prepared.IntentID == "" || prepared.IntentFingerprint == "" ||
		prepared.IdempotencyKey == "" || prepared.Approval != agentbroker.ApprovalPerCommit ||
		prepared.ExpiresAt == nil || !now.UTC().Before(*prepared.ExpiresAt) ||
		now.UTC().Before(approval.Payload.Window.NotBefore) || !now.UTC().Before(approval.Payload.Window.ExpiresAt) {
		return ErrInvalidApproval
	}
	return nil
}

func (approval *Approval) DecisionInput(prepared agentbroker.PreparedIntent) agentbroker.ApprovalInput {
	if approval == nil {
		return agentbroker.ApprovalInput{}
	}
	return agentbroker.ApprovalInput{ApprovalID: approval.Payload.ApprovalID,
		UserID: prepared.Subject.UserID, IntentID: prepared.IntentID,
		IntentFingerprint: prepared.IntentFingerprint, Decision: "approved", ActorType: "operator",
		ActorID: approval.Payload.Actor.ID, ReasonCode: approval.Payload.Actor.ReasonCode, ExpectedRevision: 1}
}

func (approval *Approval) DecisionInputForResult(userID string, prepared agentrunner.PrepareResult) agentbroker.ApprovalInput {
	if approval == nil {
		return agentbroker.ApprovalInput{}
	}
	return agentbroker.ApprovalInput{ApprovalID: approval.Payload.ApprovalID, UserID: userID,
		IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		Decision: "approved", ActorType: "operator", ActorID: approval.Payload.Actor.ID,
		ReasonCode: approval.Payload.Actor.ReasonCode, ExpectedRevision: 1}
}

func validateApprovalPayload(value ApprovalPayload, binding ApprovalBinding, now time.Time) error {
	if value.SchemaVersion != ApprovalSchemaVersion || !approvalIDPattern.MatchString(value.ApprovalID) ||
		value.Decision != "approved" || !commitPattern.MatchString(value.Release.GitCommit) ||
		value.Release.GitCommit != binding.ReleaseCommit || value.Release.MigrationHead != 94 ||
		value.Target.DeploymentFingerprint != binding.TargetFingerprint || value.Target.RunnerID != binding.RunnerID ||
		value.Activation.Stage != ApprovalStage || value.Activation.ActivationFingerprint != binding.ActivationFingerprint ||
		value.Activation.PlanFingerprint != binding.PlanFingerprint ||
		value.Request.CallerIdentity != ProjectCanaryIdentity || value.Request.CallerIdentity != binding.CallerIdentity ||
		value.Request.RequestIdentity != binding.RequestIdentity || value.Request.IdempotencyKey != binding.IdempotencyKey ||
		value.Actor.Type != "operator" || !actorIDPattern.MatchString(value.Actor.ID) ||
		!reasonCodePattern.MatchString(value.Actor.ReasonCode) || validateApprovalAction(value.Action, binding.Action) != nil ||
		value.Window.IssuedAt.IsZero() || value.Window.NotBefore.IsZero() || value.Window.ExpiresAt.IsZero() ||
		value.Window.IssuedAt.After(value.Window.NotBefore) || !value.Window.ExpiresAt.After(value.Window.NotBefore) ||
		value.Window.ExpiresAt.Sub(value.Window.NotBefore) > maximumApprovalWindow ||
		now.Before(value.Window.NotBefore) || !now.Before(value.Window.ExpiresAt) || value.Window.IssuedAt.After(now) {
		return ErrInvalidApproval
	}
	for _, candidate := range []string{value.Target.DeploymentFingerprint,
		value.Activation.ActivationFingerprint, value.Activation.PlanFingerprint,
		value.Action.BaseRevision, value.Action.ContentFingerprint, value.Action.MutationFingerprint} {
		if !planFingerprint.MatchString(candidate) || candidate == "sha256:"+strings.Repeat("0", 64) {
			return ErrInvalidApproval
		}
	}
	if value.Release.GitCommit == strings.Repeat("0", 40) {
		return ErrInvalidApproval
	}
	return nil
}

func validateApprovalAction(value ApprovalAction, binding Binding) error {
	if value.ToolIdentity != binding.Plan.ToolIdentity || value.Capability != binding.Plan.Capability ||
		value.Action != binding.Plan.Action || value.Resource != binding.Plan.Resource ||
		value.BaseRevision != binding.Plan.BaseRevision || value.Path != binding.Plan.Project.Path ||
		value.ContentFingerprint != binding.ContentFingerprint || value.MutationFingerprint != binding.MutationFingerprint {
		return ErrInvalidApproval
	}
	return nil
}

func readApprovalFile(path string, private bool) ([]byte, error) {
	path = strings.TrimSpace(path)
	if !filepath.IsAbs(path) {
		return nil, ErrInvalidApproval
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, ErrInvalidApproval
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info == nil {
		return nil, ErrInvalidApproval
	}
	var stat unix.Stat_t
	statErr := unix.Fstat(fd, &stat)
	allowedOwner := statErr == nil && (stat.Uid == uint32(os.Geteuid()) || stat.Uid == 0)
	mode := info.Mode().Perm()
	modeAccepted := mode&0o022 == 0
	if private {
		modeAccepted = mode == 0o400 || mode == 0o600
	}
	if !info.Mode().IsRegular() || !allowedOwner || !modeAccepted ||
		info.Size() < 2 || info.Size() > maximumApprovalBytes {
		return nil, ErrInvalidApproval
	}
	raw, err := io.ReadAll(io.LimitReader(file, maximumApprovalBytes+1))
	if err != nil || int64(len(raw)) != info.Size() || len(raw) > maximumApprovalBytes {
		return nil, ErrInvalidApproval
	}
	return raw, nil
}
