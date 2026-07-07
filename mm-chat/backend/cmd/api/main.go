package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/httpserver"
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg := config.Load()
	server := httpserver.New(cfg)

	errorsCh := make(chan error, 1)
	go func() {
		log.Printf("mm-chat api listening on %s version=%s", cfg.Addr, cfg.Version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errorsCh:
		log.Fatalf("mm-chat api failed: %v", err)
	case sig := <-stopCh:
		log.Printf("mm-chat api shutting down: signal=%s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("mm-chat api shutdown failed: %v", err)
	}
}
