package localskills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestBackgroundJobCompletesWaitsAndPublishesScopedNotice(t *testing.T) {
	workspace := t.TempDir()
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	scope := JobScope{UserID: "user-1", ConversationID: "conversation-1"}
	started, err := executor.StartBackgroundJob(context.Background(), JobStartRequest{
		Scope: scope, Command: Request{Command: "sleep 0.05; printf job-done"},
	})
	if err != nil || started.ID == "" || started.Status != JobStatusRunning ||
		started.Stdout != "" || started.ExitCode != nil {
		t.Fatalf("started=%#v error=%v", started, err)
	}
	completed, err := executor.BackgroundJobOutput(
		context.Background(), scope, started.ID, true, time.Second,
	)
	if err != nil || completed.Status != JobStatusCompleted ||
		completed.ExitCode == nil || *completed.ExitCode != 0 || completed.Stdout != "job-done" {
		t.Fatalf("completed=%#v error=%v", completed, err)
	}
	listed, err := executor.ListBackgroundJobs(scope)
	if err != nil || len(listed) != 1 || listed[0].ID != started.ID ||
		listed[0].Stdout != "" || listed[0].ExitCode != nil {
		t.Fatalf("listed=%#v error=%v", listed, err)
	}
	notices := executor.ConsumeJobNotices(scope)
	if len(notices) != 1 || notices[0].ID != started.ID ||
		notices[0].Status != JobStatusCompleted || len(executor.ConsumeJobNotices(scope)) != 0 {
		t.Fatalf("notices=%#v", notices)
	}
}

func TestBackgroundJobAuthorizationCollapsesCrossScopeToNotFound(t *testing.T) {
	executor := newTestExecutor(t, t.TempDir(), ApprovalSmart, 64<<10, 3*time.Second)
	owner := JobScope{UserID: "user-1", ConversationID: "conversation-1"}
	started, err := executor.StartBackgroundJob(context.Background(), JobStartRequest{
		Scope: owner, Command: Request{Command: "sleep 0.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forged := range []JobScope{
		{UserID: "user-2", ConversationID: owner.ConversationID},
		{UserID: owner.UserID, ConversationID: "conversation-2"},
	} {
		if _, err := executor.BackgroundJobOutput(
			context.Background(), forged, started.ID, false, 0,
		); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("forged scope=%#v output error=%v", forged, err)
		}
		if _, err := executor.KillBackgroundJob(forged, started.ID); !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("forged scope=%#v kill error=%v", forged, err)
		}
	}
	_, _ = executor.KillBackgroundJob(owner, started.ID)
}

func TestBackgroundJobKillReapsCompleteProcessGroup(t *testing.T) {
	workspace := t.TempDir()
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	scope := JobScope{UserID: "user-1", ConversationID: "conversation-1"}
	started, err := executor.StartBackgroundJob(context.Background(), JobStartRequest{
		Scope:   scope,
		Command: Request{Command: "sleep 30 & child=$!; printf '%s' \"$child\" > child.pid; wait"},
	})
	if err != nil {
		t.Fatal(err)
	}
	pidPath := filepath.Join(workspace, "child.pid")
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(pidPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background child pid was not published")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopping, err := executor.KillBackgroundJob(scope, started.ID)
	if err != nil || stopping.Status != JobStatusStopping {
		t.Fatalf("stopping=%#v error=%v", stopping, err)
	}
	killed, err := executor.BackgroundJobOutput(
		context.Background(), scope, started.ID, true, time.Second,
	)
	if err != nil || killed.Status != JobStatusKilled || killed.FailureCategory != "killed" {
		t.Fatalf("killed=%#v error=%v", killed, err)
	}
	encodedPID, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(string(encodedPID))
	if err != nil {
		t.Fatal(err)
	}
	waitForProcessGone(t, childPID)
}

func TestBackgroundJobsShareExecutorConcurrencyAndCloseRejectsNewJobs(t *testing.T) {
	executor := newTestExecutor(t, t.TempDir(), ApprovalSmart, 64<<10, 3*time.Second)
	scope := JobScope{UserID: "user-1", ConversationID: "conversation-1"}
	started, err := executor.StartBackgroundJob(context.Background(), JobStartRequest{
		Scope: scope, Command: Request{Command: "sleep 30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), Request{Command: "printf forbidden"}); !errors.Is(err, ErrRuntimeBusy) {
		t.Fatalf("foreground did not share slot: %v", err)
	}
	if err := executor.Close(); err != nil {
		t.Fatal(err)
	}
	closed, err := executor.BackgroundJobOutput(
		context.Background(), scope, started.ID, false, 0,
	)
	if err != nil || closed.Status != JobStatusKilled {
		t.Fatalf("closed=%#v error=%v", closed, err)
	}
	if _, err := executor.StartBackgroundJob(context.Background(), JobStartRequest{
		Scope: scope, Command: Request{Command: "printf no"},
	}); !errors.Is(err, ErrExecutorClosed) {
		t.Fatalf("start after close error=%v", err)
	}
}
