package agentbroker

import (
	"context"
	"encoding/json"
)

type MCPCallResult struct {
	SanitizedResult []byte
	PossibleSend    bool
}

// MCPCommitter adapts an already-authorized MCP execution boundary. It must
// not use Conversation MCP rows as effect authority.
type MCPCommitter interface {
	CommitMCP(context.Context, string, string, map[string]any) (MCPCallResult, error)
	MCPStatus(context.Context, string) (MCPCallResult, error)
}

type MCPExecutor struct{ committer MCPCommitter }

func NewMCPExecutor(committer MCPCommitter) (*MCPExecutor, error) {
	if committer == nil {
		return nil, ErrInvalidInput
	}
	return &MCPExecutor{committer: committer}, nil
}

func (executor *MCPExecutor) Commit(ctx context.Context, request ExecutionRequest) (ExecutorReceipt, error) {
	if executor == nil || executor.committer == nil {
		return ExecutorReceipt{}, ErrExecutorUnavailable
	}
	var arguments map[string]any
	if err := json.Unmarshal(request.Arguments, &arguments); err != nil || arguments == nil {
		return ExecutorReceipt{}, &DispatchError{Cause: ErrInvalidInput, PossibleSend: false}
	}
	result, err := executor.committer.CommitMCP(ctx, request.IdempotencyKey, request.Action, arguments)
	if err != nil {
		return ExecutorReceipt{}, &DispatchError{Cause: err, PossibleSend: result.PossibleSend}
	}
	canonical, err := canonicalJSON(result.SanitizedResult, maxArgumentsBytes)
	if err != nil {
		return ExecutorReceipt{}, &DispatchError{Cause: ErrExecutorUnavailable, PossibleSend: true}
	}
	return ExecutorReceipt{ReceiptFingerprint: fingerprint("neo-mcp-executor-receipt-v1", canonical)}, nil
}

func (executor *MCPExecutor) Status(ctx context.Context, request ExecutionRequest) (ExecutorStatus, error) {
	if executor == nil || executor.committer == nil {
		return ExecutorStatus{}, ErrExecutorUnavailable
	}
	result, err := executor.committer.MCPStatus(ctx, request.IdempotencyKey)
	if err != nil {
		return ExecutorStatus{}, err
	}
	canonical, err := canonicalJSON(result.SanitizedResult, maxArgumentsBytes)
	if err != nil {
		return ExecutorStatus{}, ErrExecutorUnavailable
	}
	return ExecutorStatus{Outcome: OutcomeCommitted,
		ReceiptFingerprint: fingerprint("neo-mcp-executor-receipt-v1", canonical)}, nil
}
