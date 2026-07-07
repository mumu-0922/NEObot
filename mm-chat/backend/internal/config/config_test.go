package config

import (
	"testing"
	"time"
)

func TestLoadFromEnvDefaults(t *testing.T) {
	cfg := LoadFromEnv(func(string) (string, bool) {
		return "", false
	})

	if cfg.Addr != DefaultAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, DefaultAddr)
	}
	if cfg.Version != DefaultVersion {
		t.Fatalf("Version = %q, want %q", cfg.Version, DefaultVersion)
	}
	if cfg.DatabaseURL != "" {
		t.Fatalf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.DBMaxOpenConns != DefaultDBMaxOpenConns {
		t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, DefaultDBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != DefaultDBMaxIdleConns {
		t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, DefaultDBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != DefaultDBConnMaxLifetime {
		t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, DefaultDBConnMaxLifetime)
	}
	if cfg.Provider.Type != "" {
		t.Fatalf("Provider.Type = %q, want empty", cfg.Provider.Type)
	}
	if cfg.Provider.BaseURL != "" {
		t.Fatalf("Provider.BaseURL = %q, want empty", cfg.Provider.BaseURL)
	}
	if cfg.Provider.Model != "" {
		t.Fatalf("Provider.Model = %q, want empty", cfg.Provider.Model)
	}
	if cfg.Provider.APIKey != "" {
		t.Fatalf("Provider.APIKey = %q, want empty", cfg.Provider.APIKey)
	}
	if cfg.Provider.Timeout != DefaultProviderTimeout {
		t.Fatalf("Provider.Timeout = %s, want %s", cfg.Provider.Timeout, DefaultProviderTimeout)
	}
}

func TestLoadFromEnvOverrides(t *testing.T) {
	values := map[string]string{
		EnvAddr:              "127.0.0.1:9090",
		EnvVersion:           "test-version",
		EnvDatabaseURL:       " postgres://user:pass@localhost:5432/mmchat?sslmode=disable ",
		EnvDBMaxOpenConns:    "12",
		EnvDBMaxIdleConns:    "7",
		EnvDBConnMaxLifetime: "45m",
		EnvProviderType:      " openai_compatible ",
		EnvProviderBaseURL:   " https://sub.example.test/v1/ ",
		EnvProviderModel:     " gpt-5.5 ",
		EnvProviderAPIKey:    " secret-key ",
		EnvProviderTimeout:   "90s",
	}

	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.Addr != "127.0.0.1:9090" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, "127.0.0.1:9090")
	}
	if cfg.Version != "test-version" {
		t.Fatalf("Version = %q, want %q", cfg.Version, "test-version")
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/mmchat?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.DBMaxOpenConns != 12 {
		t.Fatalf("DBMaxOpenConns = %d, want 12", cfg.DBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != 7 {
		t.Fatalf("DBMaxIdleConns = %d, want 7", cfg.DBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != 45*time.Minute {
		t.Fatalf("DBConnMaxLifetime = %s, want 45m", cfg.DBConnMaxLifetime)
	}
	if cfg.Provider.Type != "openai_compatible" {
		t.Fatalf("Provider.Type = %q, want openai_compatible", cfg.Provider.Type)
	}
	if cfg.Provider.BaseURL != "https://sub.example.test/v1/" {
		t.Fatalf("Provider.BaseURL = %q", cfg.Provider.BaseURL)
	}
	if cfg.Provider.Model != "gpt-5.5" {
		t.Fatalf("Provider.Model = %q, want gpt-5.5", cfg.Provider.Model)
	}
	if cfg.Provider.APIKey != "secret-key" {
		t.Fatalf("Provider.APIKey = %q, want secret-key", cfg.Provider.APIKey)
	}
	if cfg.Provider.Timeout != 90*time.Second {
		t.Fatalf("Provider.Timeout = %s, want 90s", cfg.Provider.Timeout)
	}
}

func TestLoadFromEnvIgnoresBlankValues(t *testing.T) {
	values := map[string]string{
		EnvAddr:              "   ",
		EnvVersion:           "\t",
		EnvDatabaseURL:       " \n ",
		EnvDBMaxOpenConns:    " ",
		EnvDBMaxIdleConns:    "\t",
		EnvDBConnMaxLifetime: " \n",
		EnvProviderType:      " ",
		EnvProviderBaseURL:   "\t",
		EnvProviderModel:     " \n ",
		EnvProviderAPIKey:    " ",
		EnvProviderTimeout:   "\t",
	}

	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.Addr != DefaultAddr {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, DefaultAddr)
	}
	if cfg.Version != DefaultVersion {
		t.Fatalf("Version = %q, want %q", cfg.Version, DefaultVersion)
	}
	if cfg.DatabaseURL != "" {
		t.Fatalf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
	if cfg.DBMaxOpenConns != DefaultDBMaxOpenConns {
		t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, DefaultDBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != DefaultDBMaxIdleConns {
		t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, DefaultDBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != DefaultDBConnMaxLifetime {
		t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, DefaultDBConnMaxLifetime)
	}
	if cfg.Provider.Type != "" || cfg.Provider.BaseURL != "" ||
		cfg.Provider.Model != "" || cfg.Provider.APIKey != "" {
		t.Fatalf("Provider = %#v, want blank strings", cfg.Provider)
	}
	if cfg.Provider.Timeout != DefaultProviderTimeout {
		t.Fatalf("Provider.Timeout = %s, want %s", cfg.Provider.Timeout, DefaultProviderTimeout)
	}
}

func TestLoadFromEnvFallsBackForInvalidDBValues(t *testing.T) {
	values := map[string]string{
		EnvDBMaxOpenConns:    "not-an-int",
		EnvDBMaxIdleConns:    "-1",
		EnvDBConnMaxLifetime: "not-a-duration",
		EnvProviderTimeout:   "-1s",
	}

	cfg := LoadFromEnv(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})

	if cfg.DBMaxOpenConns != DefaultDBMaxOpenConns {
		t.Fatalf("DBMaxOpenConns = %d, want %d", cfg.DBMaxOpenConns, DefaultDBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns != DefaultDBMaxIdleConns {
		t.Fatalf("DBMaxIdleConns = %d, want %d", cfg.DBMaxIdleConns, DefaultDBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime != DefaultDBConnMaxLifetime {
		t.Fatalf("DBConnMaxLifetime = %s, want %s", cfg.DBConnMaxLifetime, DefaultDBConnMaxLifetime)
	}
	if cfg.Provider.Timeout != DefaultProviderTimeout {
		t.Fatalf("Provider.Timeout = %s, want %s", cfg.Provider.Timeout, DefaultProviderTimeout)
	}
}
