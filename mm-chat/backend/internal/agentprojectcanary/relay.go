package agentprojectcanary

import (
	"context"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

type Broker interface {
	Prepare(context.Context, agentbroker.PrepareInput) (agentbroker.PreparedIntent, error)
	DecideApproval(context.Context, agentbroker.ApprovalInput) (agentbroker.PreparedIntent, error)
	Commit(context.Context, agentbroker.CommitInput) (agentbroker.CommitResult, error)
}

type PrepareBinding struct {
	UserID   string
	Grant    agentbroker.CapabilityGrant
	Registry agentbroker.ToolRegistry
}

type BindingResolver interface {
	ResolvePrepare(agentrunner.PrepareRequest, time.Time) (PrepareBinding, error)
	ResolveCommit(agentrunner.CommitRequest, time.Time) (string, error)
}

type RelayTarget struct {
	broker   Broker
	bindings BindingResolver
	now      func() time.Time
}

func NewRelayTarget(broker Broker, bindings BindingResolver) (*RelayTarget, error) {
	if broker == nil || bindings == nil {
		return nil, agentbroker.ErrInvalidInput
	}
	return &RelayTarget{broker: broker, bindings: bindings, now: time.Now}, nil
}

func (target *RelayTarget) Prepare(ctx context.Context, request agentrunner.PrepareRequest) (agentrunner.PrepareResult, error) {
	if target == nil || target.broker == nil || target.bindings == nil {
		return agentrunner.PrepareResult{}, agentbroker.ErrExecutorUnavailable
	}
	binding, err := target.bindings.ResolvePrepare(request, target.now().UTC())
	if err != nil {
		return agentrunner.PrepareResult{}, err
	}
	prepared, err := target.broker.Prepare(ctx, agentbroker.PrepareInput{
		RequestID: relayRequestID(request.Authority.RequestID),
		Attempt: attemptAuthority(binding.UserID, request.Attempt, request.Authority,
			request.SnapshotFingerprint, request.GrantFingerprint, request.RegistryFingerprint),
		Grant: binding.Grant, Registry: binding.Registry, ToolIdentity: request.ToolIdentity,
		Action: request.Action, Resource: request.Resource, Arguments: request.Arguments,
		BaseRevision: request.BaseRevision, TTL: time.Duration(request.TTLSeconds) * time.Second,
	})
	if err != nil {
		return agentrunner.PrepareResult{}, err
	}
	expiresAt := prepared.ExpiresAt
	return agentrunner.PrepareResult{Prepared: true, IntentID: prepared.IntentID,
		IntentFingerprint: prepared.IntentFingerprint, IdempotencyKey: prepared.IdempotencyKey,
		Approval: prepared.ApprovalClass, ExpiresAt: &expiresAt, Replay: prepared.Replay}, nil
}

func (target *RelayTarget) Commit(ctx context.Context, request agentrunner.CommitRequest) (agentrunner.CommitResult, error) {
	if target == nil || target.broker == nil || target.bindings == nil {
		return agentrunner.CommitResult{}, agentbroker.ErrExecutorUnavailable
	}
	userID, err := target.bindings.ResolveCommit(request, target.now().UTC())
	if err != nil {
		return agentrunner.CommitResult{}, err
	}
	result, err := target.broker.Commit(ctx, agentbroker.CommitInput{
		Attempt: attemptAuthority(userID, request.Attempt, request.Authority,
			request.SnapshotFingerprint, request.GrantFingerprint, request.RegistryFingerprint),
		IntentID: request.IntentID, IntentFingerprint: request.IntentFingerprint,
		ApprovalID: request.ApprovalID, IdempotencyKey: request.IdempotencyKey,
	})
	return agentrunner.CommitResult{Outcome: result.Outcome, IdempotencyKey: result.IdempotencyKey,
		ReceiptFingerprint: result.ReceiptFingerprint, Replay: result.Replay}, err
}

func relayRequestID(rpcID string) string {
	return "request_" + strings.TrimPrefix(rpcID, "rpc_")
}

func attemptAuthority(userID string, attempt agentrunner.AttemptRef, ticket agentrunner.AuthorityTicket,
	snapshot, grant, registry string,
) agentbroker.AttemptAuthority {
	return agentbroker.AttemptAuthority{UserID: userID, RunID: attempt.RunID, StepID: attempt.StepID,
		AttemptID: attempt.AttemptID, Generation: attempt.LeaseGeneration,
		LeaseOwner: attempt.LeaseOwner, LeaseToken: attempt.LeaseToken,
		SnapshotFingerprint: snapshot, GrantFingerprint: grant, RegistryFingerprint: registry,
		KillSwitchEpoch: ticket.KillSwitchEpoch}
}
