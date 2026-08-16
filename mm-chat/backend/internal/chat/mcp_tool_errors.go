package chat

import (
	"context"
	"errors"
	"strings"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

func fatalMCPExecutionError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, mcpclient.ErrOutcomeUnknown):
		return &mcpRunFailure{code: "MCP_OUTCOME_UNKNOWN", err: err}
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
		// A per-Tool deadline is a structured result for same-model recovery.
		// Only the parent Run deadline terminates the Turn.
		return nil
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, mcpclient.ErrToolBudget):
		return &mcpRunFailure{code: "MCP_BUDGET_EXHAUSTED", err: err}
	case errors.Is(err, context.Canceled):
		return &mcpRunFailure{code: "MCP_CANCELED", err: err}
	case errors.Is(err, mcpclient.ErrCredentialRequired),
		errors.Is(err, mcpclient.ErrCredentialInvalid),
		errors.Is(err, mcpclient.ErrServerNeedsAuth):
		return &mcpRunFailure{code: "MCP_AUTH_REQUIRED", err: err}
	case errors.Is(err, mcpclient.ErrServerUnavailable),
		errors.Is(err, mcpclient.ErrServerNotReady),
		errors.Is(err, mcpclient.ErrRemoteDisabled),
		errors.Is(err, mcpclient.ErrStdioDisabled),
		errors.Is(err, mcpclient.ErrDisabled):
		return &mcpRunFailure{code: "MCP_SERVER_UNAVAILABLE", err: err}
	case errors.Is(err, mcpclient.ErrSelectionInvalid),
		errors.Is(err, mcpclient.ErrToolNotFound):
		return &mcpRunFailure{code: "MCP_AUTHORIZATION_FAILED", err: err}
	default:
		return nil
	}
}

func nonEmptyMCPFailure(category string, err error) string {
	if category = strings.TrimSpace(category); category != "" {
		return category
	}
	if errors.Is(err, mcpclient.ErrToolArgumentsInvalid) {
		return "arguments_invalid"
	}
	if errors.Is(err, mcpclient.ErrResponseTooLarge) {
		return "result_too_large"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "tool_timeout"
	}
	return "tool_failed"
}
