package agentbroker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMCPExecutorMutablePossibleSendNeverRetries(t *testing.T) {
	committer := &fakeMCPCommitter{commitResult: MCPCallResult{PossibleSend: true}, commitErr: errors.New("ack lost"),
		statusResult: MCPCallResult{SanitizedResult: []byte(`{"ok":true}`)}}
	executor, _ := NewMCPExecutor(committer)
	repository := &memoryRepository{intents: map[string]PreparedIntent{}, requests: map[string]string{}, approvals: map[string]ApprovalInput{}}
	now := testPrepareInput(t, time.Now().UTC(), ApprovalAutomatic)
	service, _ := NewService(repository, map[string]EffectExecutor{"workspace_read": executor})
	service.now = func() time.Time { return now.Grant.IssuedAt.Add(time.Minute) }
	prepared, err := service.Prepare(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Commit(context.Background(), testCommitInput(prepared, now.Attempt))
	if err != nil || result.Outcome != OutcomeCommitted || committer.commits != 1 || committer.statuses != 1 {
		t.Fatalf("Commit = %#v, %v; calls=%d/%d", result, err, committer.commits, committer.statuses)
	}
}

type fakeMCPCommitter struct {
	commits, statuses          int
	commitResult, statusResult MCPCallResult
	commitErr, statusErr       error
}

func (committer *fakeMCPCommitter) CommitMCP(context.Context, string, string, map[string]any) (MCPCallResult, error) {
	committer.commits++
	return committer.commitResult, committer.commitErr
}
func (committer *fakeMCPCommitter) MCPStatus(context.Context, string) (MCPCallResult, error) {
	committer.statuses++
	return committer.statusResult, committer.statusErr
}
