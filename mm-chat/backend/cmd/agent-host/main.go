package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/localskills"
)

const (
	envRunnerID   = "AGENT_HOST_RUNNER_ID"
	envSocket     = "AGENT_HOST_SOCKET"
	envTokenFile  = "AGENT_HOST_TOKEN_FILE"
	envSkillsRoot = "AGENT_HOST_SKILLS_ROOT"
)

func main() {
	if err := run(); err != nil {
		log.Print("agent Host startup failed")
		os.Exit(1)
	}
}

func run() error {
	if os.Geteuid() == 0 {
		return errors.New("agent Host refuses root execution")
	}
	runnerID := strings.TrimSpace(os.Getenv(envRunnerID))
	token, err := agenthost.LoadTokenFile(os.Getenv(envTokenFile))
	if err != nil {
		return err
	}
	resolver, err := agenthost.NewLocalWorkspaceResolver(runnerID)
	if err != nil {
		return err
	}
	execution, err := agenthost.NewExecutionManager(resolver, agenthost.ExecutionConfig{
		SkillsRoot: os.Getenv(envSkillsRoot), ShellPath: "/bin/bash",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: 30 * time.Second,
		RunTimeout: 5 * time.Minute, MaxOutput: 1 << 20, MaxConcurrent: 2,
	})
	if err != nil {
		return err
	}
	defer execution.Close()
	handler, err := agenthost.NewHandler(agenthost.HandlerConfig{
		RunnerID:  runnerID,
		Version:   os.Getenv("MM_CHAT_VERSION"),
		Token:     token,
		Resolver:  resolver,
		Execution: execution,
	})
	if err != nil {
		return err
	}
	listener, err := agenthost.ListenUnix(os.Getenv(envSocket))
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Native Windows folder selection is an authenticated human interaction.
		// The Backend picker client remains capped at five minutes.
		WriteTimeout:   6 * time.Minute,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
