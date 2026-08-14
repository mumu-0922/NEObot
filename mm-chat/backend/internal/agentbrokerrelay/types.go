// Package agentbrokerrelay implements the private mTLS hop from the
// credential-free host Runner to the G21.2 Broker canary service.
package agentbrokerrelay

import (
	"context"
	"encoding/json"
)

import "neo-chat/mm-chat/backend/internal/agentrunner"

const (
	ProtocolVersion = "neo.agent-broker-relay/v1"
	Path            = "/internal/agent-broker/v1/relay"
)

type envelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	Method        string          `json:"method"`
	Body          json.RawMessage `json:"body"`
}

type response struct {
	SchemaVersion string                     `json:"schemaVersion"`
	Method        string                     `json:"method"`
	Prepare       *agentrunner.PrepareResult `json:"prepare,omitempty"`
	Commit        *agentrunner.CommitResult  `json:"commit,omitempty"`
	Error         *agentrunner.RPCError      `json:"error,omitempty"`
}

type Target interface {
	Prepare(context.Context, agentrunner.PrepareRequest) (agentrunner.PrepareResult, error)
	Commit(context.Context, agentrunner.CommitRequest) (agentrunner.CommitResult, error)
}
