package localskills

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

const (
	JobStatusRunning   = "running"
	JobStatusStopping  = "stopping"
	JobStatusCompleted = "completed"
	JobStatusKilled    = "killed"
	JobStatusFailed    = "failed"

	MaxBackgroundJobs         = 128
	MaxBackgroundJobsPerScope = 32
	MaxJobOutputWait          = 10 * time.Second
)

var (
	ErrJobScopeInvalid = errors.New("background Job scope is invalid")
	ErrJobNotFound     = errors.New("background Job was not found")
	ErrJobLimit        = errors.New("background Job limit reached")
	ErrExecutorClosed  = errors.New("local Skill executor is closed")
)

type JobScope struct {
	UserID         string
	ConversationID string
}

type JobStartRequest struct {
	Scope   JobScope
	Command Request
}

type JobSnapshot struct {
	ID              string `json:"jobId"`
	Status          string `json:"status"`
	StartedAt       string `json:"startedAt"`
	CompletedAt     string `json:"completedAt,omitempty"`
	ExitCode        *int   `json:"exitCode,omitempty"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	TimedOut        bool   `json:"timedOut,omitempty"`
	Truncated       bool   `json:"truncated,omitempty"`
	DurationMillis  int64  `json:"durationMillis,omitempty"`
	FailureCategory string `json:"failureCategory,omitempty"`
}

type JobNotice struct {
	ID     string `json:"jobId"`
	Status string `json:"status"`
}

type backgroundJob struct {
	id          string
	scope       JobScope
	status      string
	startedAt   time.Time
	completedAt time.Time
	result      Result
	failure     string
	cancel      context.CancelFunc
	done        chan struct{}
	killAsked   bool
}

func (executor *Executor) StartBackgroundJob(
	ctx context.Context,
	request JobStartRequest,
) (JobSnapshot, error) {
	if !executor.Enabled() {
		return JobSnapshot{}, ErrRuntimeFailed
	}
	if err := ctx.Err(); err != nil {
		return JobSnapshot{}, err
	}
	request.Scope.UserID = strings.TrimSpace(request.Scope.UserID)
	request.Scope.ConversationID = strings.TrimSpace(request.Scope.ConversationID)
	if request.Scope.UserID == "" || request.Scope.ConversationID == "" {
		return JobSnapshot{}, ErrJobScopeInvalid
	}
	workingDir, timeout, err := executor.prepareRequest(
		&request.Command, executor.config.RunTimeout,
	)
	if err != nil {
		return JobSnapshot{}, err
	}
	if !executor.acquireSlot() {
		return JobSnapshot{}, ErrRuntimeBusy
	}
	jobID, err := newBackgroundJobID()
	if err != nil {
		executor.releaseSlot()
		return JobSnapshot{}, ErrRuntimeFailed
	}
	jobCtx, cancel := context.WithCancel(executor.lifecycleCtx)
	job := &backgroundJob{
		id: jobID, scope: request.Scope, status: JobStatusRunning,
		startedAt: time.Now().UTC(), cancel: cancel, done: make(chan struct{}),
	}
	executor.jobMu.Lock()
	if executor.closed {
		executor.jobMu.Unlock()
		cancel()
		executor.releaseSlot()
		return JobSnapshot{}, ErrExecutorClosed
	}
	executor.pruneCompletedJobsLocked()
	if len(executor.jobs) >= MaxBackgroundJobs ||
		executor.scopeJobCountLocked(request.Scope) >= MaxBackgroundJobsPerScope {
		executor.jobMu.Unlock()
		cancel()
		executor.releaseSlot()
		return JobSnapshot{}, ErrJobLimit
	}
	executor.jobs[jobID] = job
	snapshot := job.snapshot(false)
	executor.jobMu.Unlock()

	go executor.runBackgroundJob(jobCtx, job, request.Command, workingDir, timeout)
	return snapshot, nil
}

func (executor *Executor) runBackgroundJob(
	ctx context.Context,
	job *backgroundJob,
	request Request,
	workingDir string,
	timeout time.Duration,
) {
	defer executor.releaseSlot()
	result, err := executor.executeReserved(ctx, request, workingDir, timeout)
	executor.jobMu.Lock()
	defer executor.jobMu.Unlock()
	job.completedAt = time.Now().UTC()
	job.result = result
	switch {
	case job.killAsked || errors.Is(err, context.Canceled):
		job.status = JobStatusKilled
		job.failure = "killed"
	case err != nil:
		job.status = JobStatusFailed
		job.failure = backgroundJobFailureCategory(err)
	case result.TimedOut:
		job.status = JobStatusFailed
		job.failure = "timeout"
	case result.ExitCode != 0:
		job.status = JobStatusFailed
		job.failure = "nonzero_exit"
	default:
		job.status = JobStatusCompleted
	}
	job.cancel()
	close(job.done)
	key := jobScopeKey(job.scope)
	executor.jobNotices[key] = append(executor.jobNotices[key], JobNotice{
		ID: job.id, Status: job.status,
	})
}

func (executor *Executor) ListBackgroundJobs(scope JobScope) ([]JobSnapshot, error) {
	if !validJobScope(scope) {
		return nil, ErrJobScopeInvalid
	}
	executor.jobMu.Lock()
	defer executor.jobMu.Unlock()
	result := make([]JobSnapshot, 0)
	for _, job := range executor.jobs {
		if sameJobScope(job.scope, scope) {
			result = append(result, job.snapshot(false))
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].StartedAt > result[right].StartedAt
	})
	return result, nil
}

func (executor *Executor) BackgroundJobOutput(
	ctx context.Context,
	scope JobScope,
	jobID string,
	wait bool,
	waitTimeout time.Duration,
) (JobSnapshot, error) {
	job, err := executor.authorizedJob(scope, jobID)
	if err != nil {
		return JobSnapshot{}, err
	}
	if wait {
		if waitTimeout <= 0 || waitTimeout > MaxJobOutputWait {
			return JobSnapshot{}, ErrWorkspaceInvalidInput
		}
		timer := time.NewTimer(waitTimeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return JobSnapshot{}, ctx.Err()
		case <-job.done:
		case <-timer.C:
		}
	}
	executor.jobMu.Lock()
	defer executor.jobMu.Unlock()
	current, ok := executor.jobs[job.id]
	if !ok || !sameJobScope(current.scope, scope) {
		return JobSnapshot{}, ErrJobNotFound
	}
	return current.snapshot(true), nil
}

func (executor *Executor) KillBackgroundJob(
	scope JobScope,
	jobID string,
) (JobSnapshot, error) {
	job, err := executor.authorizedJob(scope, jobID)
	if err != nil {
		return JobSnapshot{}, err
	}
	executor.jobMu.Lock()
	defer executor.jobMu.Unlock()
	current, ok := executor.jobs[job.id]
	if !ok || !sameJobScope(current.scope, scope) {
		return JobSnapshot{}, ErrJobNotFound
	}
	if current.status == JobStatusRunning {
		current.status = JobStatusStopping
		current.killAsked = true
		current.cancel()
	}
	return current.snapshot(false), nil
}

func (executor *Executor) ConsumeJobNotices(scope JobScope) []JobNotice {
	if !validJobScope(scope) {
		return nil
	}
	executor.jobMu.Lock()
	defer executor.jobMu.Unlock()
	key := jobScopeKey(scope)
	notices := append([]JobNotice(nil), executor.jobNotices[key]...)
	delete(executor.jobNotices, key)
	return notices
}

// Close cancels and reaps every process-local Job. Jobs deliberately do not
// survive a Backend restart, and no descendant may remain after shutdown.
func (executor *Executor) Close() error {
	if executor == nil {
		return nil
	}
	executor.closeOnce.Do(func() {
		executor.jobMu.Lock()
		executor.closed = true
		executor.lifecycleCancel()
		executor.jobMu.Unlock()
	})
	executor.jobMu.Lock()
	done := make([]<-chan struct{}, 0, len(executor.jobs))
	for _, job := range executor.jobs {
		if job.status == JobStatusRunning || job.status == JobStatusStopping {
			done = append(done, job.done)
		}
	}
	executor.jobMu.Unlock()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for _, channel := range done {
		select {
		case <-channel:
		case <-timer.C:
			return ErrRuntimeFailed
		}
	}
	return nil
}

func (executor *Executor) authorizedJob(scope JobScope, jobID string) (*backgroundJob, error) {
	scope.UserID = strings.TrimSpace(scope.UserID)
	scope.ConversationID = strings.TrimSpace(scope.ConversationID)
	jobID = strings.TrimSpace(jobID)
	if !validJobScope(scope) || jobID == "" {
		return nil, ErrJobScopeInvalid
	}
	executor.jobMu.Lock()
	defer executor.jobMu.Unlock()
	job, ok := executor.jobs[jobID]
	if !ok || !sameJobScope(job.scope, scope) {
		// Deliberately collapse absence and authorization denial.
		return nil, ErrJobNotFound
	}
	return job, nil
}

func (executor *Executor) pruneCompletedJobsLocked() {
	if len(executor.jobs) < MaxBackgroundJobs {
		return
	}
	type candidate struct {
		id          string
		completedAt time.Time
	}
	candidates := make([]candidate, 0)
	for id, job := range executor.jobs {
		if job.status != JobStatusRunning && job.status != JobStatusStopping {
			candidates = append(candidates, candidate{id: id, completedAt: job.completedAt})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].completedAt.Before(candidates[right].completedAt)
	})
	for len(executor.jobs) >= MaxBackgroundJobs && len(candidates) > 0 {
		delete(executor.jobs, candidates[0].id)
		candidates = candidates[1:]
	}
}

func (executor *Executor) scopeJobCountLocked(scope JobScope) int {
	count := 0
	for _, job := range executor.jobs {
		if sameJobScope(job.scope, scope) &&
			(job.status == JobStatusRunning || job.status == JobStatusStopping) {
			count++
		}
	}
	return count
}

func (job *backgroundJob) snapshot(includeOutput bool) JobSnapshot {
	snapshot := JobSnapshot{
		ID: job.id, Status: job.status, StartedAt: job.startedAt.Format(time.RFC3339Nano),
		FailureCategory: job.failure,
	}
	if !job.completedAt.IsZero() {
		snapshot.CompletedAt = job.completedAt.Format(time.RFC3339Nano)
	}
	if includeOutput && job.status != JobStatusRunning && job.status != JobStatusStopping {
		exitCode := job.result.ExitCode
		snapshot.ExitCode = &exitCode
		snapshot.Stdout = job.result.Stdout
		snapshot.Stderr = job.result.Stderr
		snapshot.TimedOut = job.result.TimedOut
		snapshot.Truncated = job.result.Truncated
		snapshot.DurationMillis = job.result.DurationMillis
	}
	return snapshot
}

func validJobScope(scope JobScope) bool {
	return strings.TrimSpace(scope.UserID) != "" &&
		strings.TrimSpace(scope.ConversationID) != ""
}

func sameJobScope(left, right JobScope) bool {
	return left.UserID == strings.TrimSpace(right.UserID) &&
		left.ConversationID == strings.TrimSpace(right.ConversationID)
}

func jobScopeKey(scope JobScope) string {
	return strings.TrimSpace(scope.UserID) + "\x00" + strings.TrimSpace(scope.ConversationID)
}

func newBackgroundJobID() (string, error) {
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return "job_" + hex.EncodeToString(suffix[:]), nil
}

func backgroundJobFailureCategory(err error) string {
	switch {
	case errors.Is(err, ErrCommandBlocked):
		return "command_blocked"
	case errors.Is(err, ErrApprovalRequired):
		return "approval_required"
	case errors.Is(err, ErrRuntimeBusy):
		return "runtime_busy"
	case errors.Is(err, ErrInvalidCommand):
		return "arguments_invalid"
	default:
		return "execution_failed"
	}
}
