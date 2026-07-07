package config

import "testing"

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
}

func TestLoadFromEnvOverrides(t *testing.T) {
	values := map[string]string{
		EnvAddr:    "127.0.0.1:9090",
		EnvVersion: "test-version",
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
}

func TestLoadFromEnvIgnoresBlankValues(t *testing.T) {
	values := map[string]string{
		EnvAddr:    "   ",
		EnvVersion: "\t",
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
}
