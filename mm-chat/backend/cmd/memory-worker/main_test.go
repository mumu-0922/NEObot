package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestLoadWorkerConfigDefaults(t *testing.T) {
	values := map[string]string{
		envDatabaseURL:     "postgres://worker:fixture@postgres/neo_chat",
		envProviderKeyring: "/run/secrets/provider-keyring.json",
	}
	resolved, err := loadWorkerConfig(mapLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.maxOpenConns != 4 || resolved.maxIdleConns != 2 ||
		resolved.concurrency != 2 || resolved.leaseDuration != 2*time.Minute ||
		resolved.providerTimeout != 45*time.Second ||
		resolved.redisKeyPrefix != config.DefaultRedisKeyPrefix ||
		resolved.hybridShadowEnabled || resolved.l2SceneShadowEnabled {
		t.Fatalf("config = %#v", resolved)
	}
}

func TestLoadWorkerConfigL2SceneShadowFlag(t *testing.T) {
	values := map[string]string{
		envDatabaseURL:     "postgres://worker:fixture@postgres/neo_chat",
		envProviderKeyring: "/run/secrets/provider-keyring.json",
		envL2SceneShadow:   " true ",
	}
	resolved, err := loadWorkerConfig(mapLookup(values))
	if err != nil || !resolved.l2SceneShadowEnabled {
		t.Fatalf("L2 Scene worker config = %#v/%v", resolved, err)
	}
	values[envL2SceneShadow] = "sometimes"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil ||
		!strings.Contains(err.Error(), envL2SceneShadow) {
		t.Fatalf("invalid L2 Scene flag error = %v", err)
	}
}

func TestLoadWorkerConfigHybridShadowFlag(t *testing.T) {
	values := map[string]string{
		envDatabaseURL:     "postgres://worker:fixture@postgres/neo_chat",
		envProviderKeyring: "/run/secrets/provider-keyring.json",
		envHybridShadow:    " true ",
	}
	resolved, err := loadWorkerConfig(mapLookup(values))
	if err != nil || !resolved.hybridShadowEnabled {
		t.Fatalf("hybrid worker config = %#v/%v", resolved, err)
	}
	values[envHybridShadow] = "sometimes"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil ||
		!strings.Contains(err.Error(), envHybridShadow) {
		t.Fatalf("invalid hybrid flag error = %v", err)
	}
}

func TestOptionalRedisWakeFailsOpenToPostgresPolling(t *testing.T) {
	wake := make(chan struct{}, 1)
	client, subscription := openOptionalRedisWake(
		context.Background(),
		workerConfig{redisURL: "://invalid"},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		wake,
	)
	if client != nil || subscription != nil {
		t.Fatalf("redis fallback = %#v/%#v", client, subscription)
	}
}

func TestLoadWorkerConfigRejectsUnsafeLeaseAndPool(t *testing.T) {
	tests := []map[string]string{
		{
			envDatabaseURL: "postgres://fixture", envProviderKeyring: "/fixture",
			envLeaseDuration: "30s", envProviderTimeout: "30s",
		},
		{
			envDatabaseURL: "postgres://fixture", envProviderKeyring: "/fixture",
			envMaxOpenConns: "2", envMaxIdleConns: "3",
		},
	}
	for _, values := range tests {
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil {
			t.Fatalf("values unexpectedly accepted: %#v", values)
		}
	}
}

func TestLoadWorkerConfigRequiresDedicatedDatabaseAndKeyring(t *testing.T) {
	for _, values := range []map[string]string{
		{},
		{envDatabaseURL: "postgres://fixture"},
	} {
		_, err := loadWorkerConfig(mapLookup(values))
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Fatalf("error = %v", err)
		}
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
