package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/storage"
	"neo-chat/mm-chat/backend/internal/teams"
)

func TestNewRecoveryDeliveryDisabledWhenSMTPBlank(t *testing.T) {
	delivery, err := newRecoveryDelivery(config.Config{})
	if err != nil || delivery != nil {
		t.Fatalf("newRecoveryDelivery() = %T/%v, want nil/nil", delivery, err)
	}
}

func TestNewRecoveryDeliveryValidatesConfiguredSMTP(t *testing.T) {
	secret := "mail-secret-value"
	_, err := newRecoveryDelivery(config.Config{Auth: config.AuthConfig{
		SMTP: config.SMTPRecoveryConfig{
			Addr:      "smtp.example.test:587",
			Username:  "mailer",
			Password:  secret,
			From:      "",
			QueueSize: 10,
			Timeout:   time.Second,
		},
	}})
	if err == nil {
		t.Fatal("newRecoveryDelivery() error = nil, want invalid sender error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("newRecoveryDelivery() leaked SMTP password: %v", err)
	}
}

func TestNewRedisStateDisabledWhenURLBlank(t *testing.T) {
	client, cancelStore, rateLimitStore, sessionCacheStore, err := newRedisState(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("newRedisState() error = %v", err)
	}
	if client != nil || cancelStore != nil || rateLimitStore != nil || sessionCacheStore != nil {
		t.Fatalf(
			"newRedisState() = %#v/%#v/%#v/%#v, want nil client and stores",
			client,
			cancelStore,
			rateLimitStore,
			sessionCacheStore,
		)
	}
}

func TestNewRedisStateRejectsInvalidURLWithoutSecretLeak(t *testing.T) {
	secret := "super-secret-password"
	_, _, _, _, err := newRedisState(context.Background(), config.Config{
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
	_, _, _, _, err := newRedisState(context.Background(), config.Config{
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

func TestRedactSensitiveLogText(t *testing.T) {
	input := strings.Join([]string{
		"postgres://neo_chat:super-secret@postgres:5432/neo_chat?sslmode=disable",
		"postgres://standalone-token@postgres:5432/neo_chat",
		"postgres://neo_chat:p@ss@postgres:5432/neo_chat",
		"postgres://neo_chat:slash/secret@postgres:5432/neo_chat",
		"redis://:redis-secret@redis:6379/0",
		"password=plain-secret",
		"api_key=provider-secret",
		"S3_SECRET_ACCESS_KEY=minio-secret",
		"secret_access_key=snake-secret",
		"Authorization=Bearer-token",
		"Authorization: Bearer header-secret-token",
		"Authorization=Bearer assignment-secret-token",
		"Bearer raw-bearer-token",
		"TEAM_CURSOR_KEYRING=cursor-v1=encoded-secret-key",
	}, " ")

	got := redactSensitiveLogText(input)
	for _, secret := range []string{
		"super-secret",
		"redis-secret",
		"plain-secret",
		"provider-secret",
		"minio-secret",
		"snake-secret",
		"standalone-token",
		"p@ss",
		"slash/secret",
		"header-secret-token",
		"assignment-secret-token",
		"raw-bearer-token",
		"encoded-secret-key",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text still contains %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("redacted text = %q, want redaction marker", got)
	}
}

func TestNewTeamRuntimeKeepsDeliveryDisabledWithoutPrerequisites(t *testing.T) {
	runtime, err := newTeamRuntime(nil, config.Config{})
	if err != nil {
		t.Fatalf("newTeamRuntime() error = %v", err)
	}
	if runtime == nil || runtime.service == nil {
		t.Fatal("newTeamRuntime() service = nil, want Team handler service")
	}
	if runtime.worker != nil {
		t.Fatalf("newTeamRuntime() worker = %T, want nil without DB/SMTP/keys", runtime.worker)
	}
}

func TestNewTeamRuntimeRequiresEveryInviteDeliveryPrerequisite(t *testing.T) {
	tests := []struct {
		name      string
		db        *sql.DB
		mutate    func(*config.Config)
		wantError bool
	}{
		{name: "database", db: nil, mutate: func(*config.Config) {}},
		{
			name: "smtp",
			db:   new(sql.DB),
			mutate: func(cfg *config.Config) {
				cfg.Auth.SMTP = config.SMTPRecoveryConfig{}
			},
			wantError: true,
		},
		{
			name: "mail keys",
			db:   new(sql.DB),
			mutate: func(cfg *config.Config) {
				cfg.Team.Mail = config.TeamKeyringConfig{}
			},
			wantError: true,
		},
		{
			name: "accept URL",
			db:   new(sql.DB),
			mutate: func(cfg *config.Config) {
				cfg.Team.InviteAcceptURLBase = ""
			},
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTeamRuntimeConfig()
			tt.mutate(&cfg)
			runtime, err := newTeamRuntime(tt.db, cfg)
			if tt.wantError {
				if err == nil {
					t.Fatal("newTeamRuntime() partial delivery configuration error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("newTeamRuntime() error = %v", err)
			}
			if runtime.service == nil {
				t.Fatal("newTeamRuntime() service = nil")
			}
			if runtime.worker != nil {
				t.Fatalf("newTeamRuntime() worker = %T, want disabled", runtime.worker)
			}
		})
	}
}

func TestNewTeamRuntimeWiresRotationAndFailsClosedBeforeWorkerStarts(t *testing.T) {
	cfg := validTeamRuntimeConfig()
	runtime, err := newTeamRuntime(new(sql.DB), cfg)
	if err != nil {
		t.Fatalf("newTeamRuntime() error = %v", err)
	}
	if runtime.service == nil || runtime.worker == nil {
		t.Fatalf("newTeamRuntime() = %#v, want service and worker", runtime)
	}
	if err := runtime.worker.AdmitInviteDelivery(context.Background()); !errors.Is(err, teams.ErrInviteDeliveryUnavailable) {
		t.Fatalf(
			"AdmitInviteDelivery() before Run error = %v, want ErrInviteDeliveryUnavailable",
			err,
		)
	}
	if err := (teamWorkerReadiness{gate: runtime.worker}).CheckReady(
		context.Background(),
	); !errors.Is(err, teams.ErrInviteDeliveryUnavailable) {
		t.Fatalf("team worker readiness before Run error = %v", err)
	}
}

func TestNewTeamRuntimeRejectsMalformedAndSharedKeysWithoutLeak(t *testing.T) {
	secret := "not-base64-secret-key-material!"
	cfg := config.Config{Team: config.TeamConfig{
		Cursor: config.TeamKeyringConfig{
			ActiveKeyID: "cursor-v1",
			Keyring:     "cursor-v1=" + secret,
		},
	}}
	_, err := newTeamRuntime(nil, cfg)
	if err == nil {
		t.Fatal("newTeamRuntime() malformed key error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("newTeamRuntime() leaked malformed key bytes: %v", err)
	}

	shared := bytes.Repeat([]byte{0x55}, 32)
	encoded := base64.StdEncoding.EncodeToString(shared)
	cfg = validTeamRuntimeConfig()
	cfg.Team.Cursor = config.TeamKeyringConfig{
		ActiveKeyID: "cursor-v1",
		Keyring:     "cursor-v1=" + encoded,
	}
	cfg.Team.Mail = config.TeamKeyringConfig{
		ActiveKeyID: "mail-v1",
		Keyring:     "mail-v1=" + encoded,
	}
	_, err = newTeamRuntime(nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "distinct key material") {
		t.Fatalf("newTeamRuntime() shared key error = %v", err)
	}
	if strings.Contains(err.Error(), encoded) {
		t.Fatalf("newTeamRuntime() leaked shared key bytes: %v", err)
	}

	providerSecret := "provider-secret-32-bytes-value!!"
	if len(providerSecret) != 32 {
		t.Fatalf("test provider secret length = %d, want 32", len(providerSecret))
	}
	cfg = config.Config{
		Provider: config.ProviderConfig{APIKey: providerSecret},
		Team: config.TeamConfig{Cursor: config.TeamKeyringConfig{
			ActiveKeyID: "cursor-v1",
			Keyring: "cursor-v1=" + base64.StdEncoding.EncodeToString(
				[]byte(providerSecret),
			),
		}},
	}
	_, err = newTeamRuntime(nil, cfg)
	if err == nil || !strings.Contains(err.Error(), config.EnvProviderAPIKey) {
		t.Fatalf("newTeamRuntime() reused provider key error = %v", err)
	}
	if strings.Contains(err.Error(), providerSecret) {
		t.Fatalf("newTeamRuntime() leaked provider/key bytes: %v", err)
	}

	for name, tt := range map[string]struct {
		field string
		apply func(*config.Config, string)
	}{
		"database password": {
			field: config.EnvDatabaseURL,
			apply: func(cfg *config.Config, secret string) {
				cfg.DatabaseURL = "postgres://user:" + secret + "@db.example.test/chat"
			},
		},
		"redis password": {
			field: config.EnvRedisURL,
			apply: func(cfg *config.Config, secret string) {
				cfg.Redis.URL = "redis://user:" + secret + "@redis.example.test/0"
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			credentialSecret := strings.Repeat("d", 32)
			cfg := config.Config{Team: config.TeamConfig{
				Cursor: config.TeamKeyringConfig{
					ActiveKeyID: "cursor-v1",
					Keyring: "cursor-v1=" + base64.StdEncoding.EncodeToString(
						[]byte(credentialSecret),
					),
				},
			}}
			tt.apply(&cfg, credentialSecret)
			_, err := newTeamRuntime(nil, cfg)
			if err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("newTeamRuntime() reused URL credential error = %v", err)
			}
			if strings.Contains(err.Error(), credentialSecret) {
				t.Fatalf("newTeamRuntime() leaked URL credential bytes: %v", err)
			}
		})
	}
}

func TestNewTeamRuntimeRejectsPublishedExampleKeysInRequiredMode(t *testing.T) {
	cfg := validTeamRuntimeConfig()
	cfg.Auth.Mode = config.AuthModeRequired
	cfg.Team.Cursor = config.TeamKeyringConfig{
		ActiveKeyID: "cursor-example-v1",
		Keyring: "cursor-example-v1=" + base64.StdEncoding.EncodeToString(
			[]byte("fake-cursor-key-not-production!!"),
		),
	}

	_, err := newTeamRuntime(nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "example key material") {
		t.Fatalf("newTeamRuntime() published example key error = %v", err)
	}
	if strings.Contains(err.Error(), "fake-cursor-key-not-production") {
		t.Fatalf("newTeamRuntime() leaked example key bytes: %v", err)
	}
}

func TestNewTeamRuntimeRejectsPartialKeyringAndSMTP(t *testing.T) {
	_, err := newTeamRuntime(nil, config.Config{Team: config.TeamConfig{
		Mail: config.TeamKeyringConfig{ActiveKeyID: "mail-v1"},
	}})
	if err == nil || !strings.Contains(err.Error(), config.EnvTeamMailKeyring) {
		t.Fatalf("newTeamRuntime() partial keyring error = %v", err)
	}

	_, err = newTeamRuntime(nil, config.Config{Auth: config.AuthConfig{
		SMTP: config.SMTPRecoveryConfig{
			Addr:    "smtp.example.test:587",
			Timeout: time.Second,
		},
	}})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "sender") {
		t.Fatalf("newTeamRuntime() partial SMTP error = %v", err)
	}
}

func TestNewInviteURLBuilderValidatesBaseAndEscapesToken(t *testing.T) {
	builder, err := newInviteURLBuilder(
		"https://chat.example.test/invites/accept?source=email",
		true,
	)
	if err != nil {
		t.Fatalf("newInviteURLBuilder() error = %v", err)
	}
	token := strings.Repeat("a", 64)
	got, err := builder(token)
	if err != nil {
		t.Fatalf("invite URL builder error = %v", err)
	}
	if got != "https://chat.example.test/invites/accept?source=email#token="+token {
		t.Fatalf("invite URL = %q", got)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse invite URL: %v", err)
	}
	if strings.Contains(parsed.EscapedPath(), token) ||
		strings.Contains(parsed.RawQuery, token) ||
		parsed.Query().Get("token") != "" {
		t.Fatalf("invite URL sends raw token to the HTTP server: %q", got)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil || fragment.Get("token") != token {
		t.Fatalf("invite URL fragment = %q, err=%v", parsed.Fragment, err)
	}

	for _, invalid := range []string{
		"/relative",
		"ftp://chat.example.test/invites/accept",
		"https://user:password@chat.example.test/invites/accept",
		"https://chat.example.test/invites/accept#token",
		"https://chat.example.test/invites/accept?token=preconfigured",
	} {
		_, err := newInviteURLBuilder(invalid, true)
		if err == nil {
			t.Fatalf("newInviteURLBuilder(%q) error = nil", invalid)
		}
		if strings.Contains(err.Error(), invalid) {
			t.Fatalf("newInviteURLBuilder() leaked URL value: %v", err)
		}
	}
	if _, err := newInviteURLBuilder(
		"http://chat.example.test/invites/accept",
		false,
	); err == nil {
		t.Fatal("newInviteURLBuilder() accepted non-loopback HTTP in development")
	}
	if _, err := newInviteURLBuilder(
		"http://127.0.0.1:3000/invites/accept",
		false,
	); err != nil {
		t.Fatalf("newInviteURLBuilder() loopback development HTTP error = %v", err)
	}
	if _, err := newInviteURLBuilder(
		"http://127.0.0.1:3000/invites/accept",
		true,
	); err == nil {
		t.Fatal("newInviteURLBuilder() accepted HTTP in required mode")
	}
}

func TestRunTeamWorkerReportsUnexpectedExitAndHonorsCancellation(t *testing.T) {
	unexpected := teamWorkerFunc(func(context.Context) error { return nil })
	if err := runTeamWorker(context.Background(), unexpected); err == nil ||
		!strings.Contains(err.Error(), "unexpectedly") {
		t.Fatalf("runTeamWorker() unexpected exit error = %v", err)
	}

	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runTeamWorker(ctx, teamWorkerFunc(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}))
	}()
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runTeamWorker() cancellation error = %v", err)
	}
}

func TestRunBackgroundWorkerReportsFailuresAndHonorsCancellation(t *testing.T) {
	if err := runBackgroundWorker(context.Background(), "test worker", nil); err != nil {
		t.Fatalf("runBackgroundWorker(nil) error = %v", err)
	}

	unexpected := teamWorkerFunc(func(context.Context) error { return nil })
	if err := runBackgroundWorker(context.Background(), "test worker", unexpected); err == nil ||
		!strings.Contains(err.Error(), "test worker exited unexpectedly") {
		t.Fatalf("runBackgroundWorker() unexpected exit error = %v", err)
	}

	sentinel := errors.New("worker failure")
	failing := teamWorkerFunc(func(context.Context) error { return sentinel })
	if err := runBackgroundWorker(context.Background(), "test worker", failing); err == nil ||
		!errors.Is(err, sentinel) || !strings.Contains(err.Error(), "test worker stopped") {
		t.Fatalf("runBackgroundWorker() wrapped error = %v", err)
	}

	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runBackgroundWorker(ctx, "test worker", teamWorkerFunc(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}))
	}()
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("runBackgroundWorker() cancellation error = %v", err)
	}
}

func TestWaitForBackgroundWorkerWaitsAndTimesOut(t *testing.T) {
	if err := waitForBackgroundWorker(context.Background(), nil, "test worker"); err != nil {
		t.Fatalf("waitForBackgroundWorker(nil) error = %v", err)
	}

	done := make(chan struct{})
	close(done)
	if err := waitForBackgroundWorker(context.Background(), done, "test worker"); err != nil {
		t.Fatalf("waitForBackgroundWorker() closed error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForBackgroundWorker(ctx, make(chan struct{}), "test worker"); err == nil ||
		!strings.Contains(err.Error(), "test worker") {
		t.Fatalf("waitForBackgroundWorker() timeout error = %v", err)
	}
}

func TestWaitForTeamWorkerWaitsAndTimesOut(t *testing.T) {
	done := make(chan struct{})
	close(done)
	if err := waitForTeamWorker(context.Background(), done); err != nil {
		t.Fatalf("waitForTeamWorker() closed error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForTeamWorker(ctx, make(chan struct{})); err == nil {
		t.Fatal("waitForTeamWorker() timeout error = nil")
	}
}

func TestTeamWorkerShutdownTimeoutCoversSMTPDeadline(t *testing.T) {
	if got := teamWorkerShutdownTimeout(time.Minute); got != time.Minute+time.Second {
		t.Fatalf("teamWorkerShutdownTimeout() = %s", got)
	}
	if got := teamWorkerShutdownTimeout(time.Second); got != shutdownTimeout {
		t.Fatalf("teamWorkerShutdownTimeout(short) = %s", got)
	}
}

type teamWorkerFunc func(context.Context) error

func (f teamWorkerFunc) Run(ctx context.Context) error {
	return f(ctx)
}

func validTeamRuntimeConfig() config.Config {
	cursorActive := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	cursorOld := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x12}, 32))
	mailActive := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x21}, 32))
	mailOld := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	return config.Config{
		Auth: config.AuthConfig{SMTP: config.SMTPRecoveryConfig{
			Addr:     "smtp.example.test:587",
			Username: "mailer",
			Password: "smtp-password",
			From:     "no-reply@example.test",
			Timeout:  time.Second,
		}},
		Team: config.TeamConfig{
			Cursor: config.TeamKeyringConfig{
				ActiveKeyID: "cursor-v2",
				Keyring: "cursor-v2=" + cursorActive +
					",cursor-v1=" + cursorOld,
			},
			Mail: config.TeamKeyringConfig{
				ActiveKeyID: "mail-v2",
				Keyring: "mail-v2=" + mailActive +
					",mail-v1=" + mailOld,
			},
			InviteAcceptURLBase: "https://chat.example.test/invites/accept",
			MailWorker: config.TeamMailWorkerConfig{
				LeaseDuration:  10 * time.Second,
				PollInterval:   100 * time.Millisecond,
				BackoffBase:    time.Second,
				BackoffMaximum: time.Minute,
			},
		},
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
