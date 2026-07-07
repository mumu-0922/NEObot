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
}

func TestLoadFromEnvOverrides(t *testing.T) {
	values := map[string]string{
		EnvAddr:              "127.0.0.1:9090",
		EnvVersion:           "test-version",
		EnvDatabaseURL:       " postgres://user:pass@localhost:5432/mmchat?sslmode=disable ",
		EnvDBMaxOpenConns:    "12",
		EnvDBMaxIdleConns:    "7",
		EnvDBConnMaxLifetime: "45m",
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
}

func TestLoadFromEnvIgnoresBlankValues(t *testing.T) {
	values := map[string]string{
		EnvAddr:              "   ",
		EnvVersion:           "\t",
		EnvDatabaseURL:       " \n ",
		EnvDBMaxOpenConns:    " ",
		EnvDBMaxIdleConns:    "\t",
		EnvDBConnMaxLifetime: " \n",
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
}

func TestLoadFromEnvFallsBackForInvalidDBValues(t *testing.T) {
	values := map[string]string{
		EnvDBMaxOpenConns:    "not-an-int",
		EnvDBMaxIdleConns:    "-1",
		EnvDBConnMaxLifetime: "not-a-duration",
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
}
