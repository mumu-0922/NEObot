package main

import (
	"context"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/storage"
)

func TestNewRedisStateDisabledWhenURLBlank(t *testing.T) {
	client, cancelStore, rateLimitStore, err := newRedisState(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("newRedisState() error = %v", err)
	}
	if client != nil || cancelStore != nil || rateLimitStore != nil {
		t.Fatalf(
			"newRedisState() = %#v/%#v/%#v, want nil client and stores",
			client,
			cancelStore,
			rateLimitStore,
		)
	}
}

func TestNewRedisStateRejectsInvalidURLWithoutSecretLeak(t *testing.T) {
	secret := "super-secret-password"
	_, _, _, err := newRedisState(context.Background(), config.Config{
		Redis: config.RedisConfig{
			URL: "redis://:" + secret + "@[::1",
		},
	})
	if err == nil {
		t.Fatal("newRedisState() error = nil, want parse error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("newRedisState() error leaks secret: %v", err)
	}
}

func TestNewRedisStateRejectsEnabledRateLimitWithoutRedisURL(t *testing.T) {
	_, _, _, err := newRedisState(context.Background(), config.Config{
		Redis: config.RedisConfig{
			RateLimitEnabled: true,
		},
	})
	if err == nil {
		t.Fatal("newRedisState() error = nil, want missing REDIS_URL error")
	}
	if !strings.Contains(err.Error(), config.EnvRedisRateLimitEnabled) ||
		!strings.Contains(err.Error(), config.EnvRedisURL) {
		t.Fatalf("newRedisState() error = %v, want mention rate-limit env and redis url", err)
	}
}

func TestNewObjectStoreCreatesLocalStoreByDefault(t *testing.T) {
	store, err := newObjectStore(config.Config{
		Storage: config.StorageConfig{
			Backend:  "",
			LocalDir: t.TempDir(),
		},
	})
	if err != nil {
		t.Fatalf("newObjectStore() error = %v", err)
	}
	if _, ok := store.(*storage.LocalStore); !ok {
		t.Fatalf("store type = %T, want *storage.LocalStore", store)
	}
}

func TestNewObjectStoreCreatesS3StoreWithoutNetworkWhenAutoCreateDisabled(t *testing.T) {
	for _, backend := range []string{"minio", "s3"} {
		t.Run(backend, func(t *testing.T) {
			store, err := newObjectStore(config.Config{
				Storage: config.StorageConfig{
					Backend: backend,
					S3: config.S3Config{
						Endpoint:        "127.0.0.1:1",
						Bucket:          "neo-chat-files",
						Region:          "us-east-1",
						AccessKeyID:     "test-access",
						SecretAccessKey: "test-secret",
					},
				},
			})
			if err != nil {
				t.Fatalf("newObjectStore() error = %v", err)
			}
			if _, ok := store.(*storage.S3Store); !ok {
				t.Fatalf("store type = %T, want *storage.S3Store", store)
			}
		})
	}
}

func TestNewObjectStoreRejectsMissingS3Config(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.S3Config
		want string
	}{
		{name: "endpoint", cfg: config.S3Config{Bucket: "bucket", AccessKeyID: "key", SecretAccessKey: "secret"}, want: "endpoint"},
		{name: "bucket", cfg: config.S3Config{Endpoint: "127.0.0.1:1", AccessKeyID: "key", SecretAccessKey: "secret"}, want: "bucket"},
		{name: "access key", cfg: config.S3Config{Endpoint: "127.0.0.1:1", Bucket: "bucket", SecretAccessKey: "secret"}, want: "access key"},
		{name: "secret key", cfg: config.S3Config{Endpoint: "127.0.0.1:1", Bucket: "bucket", AccessKeyID: "key"}, want: "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newObjectStore(config.Config{
				Storage: config.StorageConfig{
					Backend: "minio",
					S3:      tt.cfg,
				},
			})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("newObjectStore() error = %v, want mention %q", err, tt.want)
			}
			if strings.Contains(err.Error(), tt.cfg.SecretAccessKey) && tt.cfg.SecretAccessKey != "" {
				t.Fatalf("newObjectStore() error leaks secret: %v", err)
			}
		})
	}
}

func TestNewObjectStoreRejectsUnsupportedBackend(t *testing.T) {
	_, err := newObjectStore(config.Config{
		Storage: config.StorageConfig{
			Backend: "ftp",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported STORAGE_BACKEND") {
		t.Fatalf("newObjectStore() error = %v, want unsupported backend", err)
	}
}
