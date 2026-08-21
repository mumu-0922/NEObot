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
)

const (
	envRunnerID  = "AGENT_HOST_RUNNER_ID"
	envSocket    = "AGENT_HOST_SOCKET"
	envTokenFile = "AGENT_HOST_TOKEN_FILE"
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
	handler, err := agenthost.NewHandler(agenthost.HandlerConfig{
		RunnerID: runnerID,
		Version:  os.Getenv("MM_CHAT_VERSION"),
		Token:    token,
		Resolver: resolver,
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
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
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
