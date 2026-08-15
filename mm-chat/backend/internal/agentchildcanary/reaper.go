package agentchildcanary

import (
	"context"
	"time"

	"neo-chat/mm-chat/backend/internal/agentdelegation"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

type RunnerClient interface {
	Call(context.Context, agentrunner.Request) (agentrunner.Response, error)
}

// RunnerReaper waits out every already signed launch authority, then removes
// exactly the terminal Child Attempt from credential-free Runner inventory.
// The durable reap is completed separately by migration 093 only after this
// method has proved the Child absent.
type RunnerReaper struct {
	runnerID string
	client   RunnerClient
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

func NewRunnerReaper(runnerID string, client RunnerClient) (*RunnerReaper, error) {
	if runnerID == "" || client == nil {
		return nil, ErrInvalidPlan
	}
	return &RunnerReaper{runnerID: runnerID, client: client, now: time.Now, wait: waitContext}, nil
}

func (reaper *RunnerReaper) Reap(ctx context.Context, target agentdelegation.ReapTarget) error {
	if reaper == nil || reaper.client == nil || target.ReapID == "" || target.ChildRunID == "" ||
		target.StepID == "" || target.AttemptID == "" || target.Generation < 1 ||
		target.LeaseOwner != reaper.runnerID || target.AuthoritySnapshotFingerprint == "" {
		return ErrUnavailable
	}
	if expiresAt := target.LaunchAuthorityExpiresAt; expiresAt != nil {
		for delay := expiresAt.Sub(reaper.now().UTC()); delay > 0; delay = expiresAt.Sub(reaper.now().UTC()) {
			if err := reaper.wait(ctx, delay); err != nil {
				return err
			}
		}
	}
	before, err := reaper.list(ctx)
	if err != nil {
		return err
	}
	expected := make([]agentrunner.SandboxDescriptor, 0, len(before))
	found := false
	for _, descriptor := range before {
		if descriptor.Attempt.RunID != target.ChildRunID {
			expected = append(expected, descriptor)
			continue
		}
		if !matchesReapTarget(descriptor, target) || found {
			return ErrUnavailable
		}
		found = true
	}
	if target.SandboxID != "" && !found {
		// The Runner may already have reaped the Child after a crash. A second
		// list below remains the decisive physical-absence proof.
		expected = append([]agentrunner.SandboxDescriptor(nil), before...)
	}
	reconcile, err := agentrunner.NewRequest(agentrunner.MethodReconcile,
		agentrunner.ReconcileRequest{RunnerID: reaper.runnerID, Expected: expected}, reaper.now().UTC())
	if err != nil {
		return ErrUnavailable
	}
	response, err := reaper.client.Call(ctx, reconcile)
	result, ok := response.Body.(agentrunner.ReconcileResult)
	if err != nil || !ok || result.RunnerID != reaper.runnerID {
		return ErrUnavailable
	}
	after, err := reaper.list(ctx)
	if err != nil {
		return err
	}
	for _, descriptor := range after {
		if descriptor.Attempt.RunID == target.ChildRunID || matchesReapTarget(descriptor, target) {
			return ErrUnavailable
		}
	}
	return nil
}

func (reaper *RunnerReaper) list(ctx context.Context) ([]agentrunner.SandboxDescriptor, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodList,
		agentrunner.ListRequest{RunnerID: reaper.runnerID}, reaper.now().UTC())
	if err != nil {
		return nil, ErrUnavailable
	}
	response, err := reaper.client.Call(ctx, request)
	result, ok := response.Body.(agentrunner.ListResult)
	if err != nil || !ok || result.RunnerID != reaper.runnerID {
		return nil, ErrUnavailable
	}
	return append([]agentrunner.SandboxDescriptor(nil), result.Sandboxes...), nil
}

func matchesReapTarget(descriptor agentrunner.SandboxDescriptor, target agentdelegation.ReapTarget) bool {
	if descriptor.Attempt.RunID != target.ChildRunID || descriptor.Attempt.StepID != target.StepID ||
		descriptor.Attempt.AttemptID != target.AttemptID ||
		descriptor.Attempt.LeaseGeneration != target.Generation ||
		descriptor.SnapshotFingerprint != target.AuthoritySnapshotFingerprint {
		return false
	}
	if target.SandboxID != "" && (descriptor.SandboxID != target.SandboxID ||
		descriptor.Attempt.LeaseGeneration != target.SandboxGeneration ||
		descriptor.SnapshotFingerprint != target.SandboxSnapshotFingerprint ||
		descriptor.SpecFingerprint != target.SpecFingerprint ||
		descriptor.ProbeFingerprint != target.ProbeFingerprint) {
		return false
	}
	return true
}

func waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
