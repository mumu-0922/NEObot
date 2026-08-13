package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/mcprunner"
)

const (
	defaultRunnerAddr    = ":8090"
	maxTokenBytes        = 4096
	runnerRequestTimeout = 2*time.Minute + 15*time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatal("mcp runner startup failed")
	}
}

func run() error {
	if os.Geteuid() == 0 {
		return errors.New("mcp runner refuses root execution")
	}
	manifestFile := strings.TrimSpace(os.Getenv("MCP_MANIFEST_FILE"))
	if manifestFile == "" {
		return errors.New("mcp manifest is required")
	}
	servers, err := mcpclient.LoadManifest(manifestFile, os.LookupEnv)
	if err != nil {
		return err
	}
	token, err := readRunnerToken(os.Getenv("MCP_RUNNER_TOKEN_FILE"))
	if err != nil {
		return err
	}
	manager, err := mcprunner.NewManager(mcprunner.Config{
		MaxProcesses:  4,
		WorkRoot:      "/work",
		ClientName:    "neo-chat-mcp-runner",
		ClientVersion: strings.TrimSpace(os.Getenv("MM_CHAT_VERSION")),
	}, servers)
	if err != nil {
		return err
	}
	defer manager.Close()
	handler, err := mcprunner.NewHandler(manager, token)
	if err != nil {
		return err
	}
	addr := strings.TrimSpace(os.Getenv("MCP_RUNNER_ADDR"))
	if addr == "" {
		addr = defaultRunnerAddr
	}
	server := &http.Server{
		Addr: addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       runnerRequestTimeout,
		WriteTimeout:      runnerRequestTimeout,
		IdleTimeout:       runnerRequestTimeout,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go manager.RunReaper(ctx)
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func readRunnerToken(path string) (string, error) {
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/run/secrets/") {
		return "", errors.New("runner token file is invalid")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("runner token is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxTokenBytes {
		return "", errors.New("runner token is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTokenBytes+1))
	if err != nil || len(data) > maxTokenBytes {
		return "", errors.New("runner token is invalid")
	}
	defer clear(data)
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return "", errors.New("runner token is invalid")
	}
	return token, nil
}
