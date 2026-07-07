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

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/httpserver"
)

const (
	databaseOpenTimeout = 5 * time.Second
	shutdownTimeout     = 10 * time.Second
)

func main() {
	cfg := config.Load()

	openCtx, openCancel := context.WithTimeout(context.Background(), databaseOpenTimeout)
	db, err := database.Open(openCtx, cfg)
	openCancel()
	if err != nil {
		log.Fatalf("mm-chat database open failed: %v", err)
	}

	var chatRepo chat.Repository
	if sqlDB := db.SQL(); sqlDB != nil {
		chatRepo = chat.NewPostgresRepository(sqlDB)
	}

	server := httpserver.New(
		cfg,
		httpserver.WithReadyChecker(db),
		httpserver.WithChatRepository(chatRepo),
	)

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
		_ = db.Close()
		log.Fatalf("mm-chat api failed: %v", err)
	case sig := <-stopCh:
		log.Printf("mm-chat api shutting down: signal=%s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		_ = db.Close()
		log.Fatalf("mm-chat api shutdown failed: %v", err)
	}
	if err := db.Close(); err != nil {
		log.Printf("mm-chat database close failed: %v", err)
	}
}
