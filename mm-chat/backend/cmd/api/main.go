package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/database"
	"neo-chat/mm-chat/backend/internal/files"
	"neo-chat/mm-chat/backend/internal/httpserver"
	"neo-chat/mm-chat/backend/internal/storage"
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
	var fileRepo files.Repository
	if sqlDB := db.SQL(); sqlDB != nil {
		chatRepo = chat.NewPostgresRepository(sqlDB)
		fileRepo = files.NewPostgresRepository(sqlDB)
	}

	chatProvider, err := newChatProvider(cfg)
	if err != nil {
		_ = db.Close()
		log.Fatalf("mm-chat provider config failed: %v", err)
	}
	if chatProvider == nil && strings.TrimSpace(cfg.Provider.Type) != "" {
		log.Printf("mm-chat provider disabled: %s requires PROVIDER_BASE_URL, PROVIDER_MODEL, and PROVIDER_API_KEY", cfg.Provider.Type)
	}

	objectStore, err := newObjectStore(cfg)
	if err != nil {
		_ = db.Close()
		log.Fatalf("mm-chat storage config failed: %v", err)
	}

	server := httpserver.New(
		cfg,
		httpserver.WithReadyChecker(db),
		httpserver.WithChatRepository(chatRepo),
		httpserver.WithChatProvider(chatProvider),
		httpserver.WithFileRepository(fileRepo),
		httpserver.WithObjectStore(objectStore),
		httpserver.WithMaxUploadBytes(cfg.Storage.MaxUploadBytes),
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

func newObjectStore(cfg config.Config) (storage.ObjectStore, error) {
	storageBackend := strings.ToLower(strings.TrimSpace(cfg.Storage.Backend))
	switch storageBackend {
	case "", "local":
		return storage.NewLocalStore(cfg.Storage.LocalDir)
	default:
		return nil, fmt.Errorf("unsupported STORAGE_BACKEND %q", cfg.Storage.Backend)
	}
}

func newChatProvider(cfg config.Config) (chat.Provider, error) {
	providerType := strings.ToLower(strings.TrimSpace(cfg.Provider.Type))
	switch providerType {
	case "", "none":
		return nil, nil
	case "openai", "openai_compatible", "openai-compatible":
		if cfg.Provider.BaseURL == "" || cfg.Provider.Model == "" || cfg.Provider.APIKey == "" {
			return nil, nil
		}

		return chat.NewOpenAICompatibleProvider(chat.OpenAICompatibleProviderConfig{
			BaseURL:      cfg.Provider.BaseURL,
			APIKey:       cfg.Provider.APIKey,
			DefaultModel: cfg.Provider.Model,
			Timeout:      cfg.Provider.Timeout,
		})
	default:
		return nil, fmt.Errorf("unsupported PROVIDER_TYPE %q", cfg.Provider.Type)
	}
}
