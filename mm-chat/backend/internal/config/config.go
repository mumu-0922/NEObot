package config

import (
	"os"
	"strings"
)

const (
	DefaultAddr    = ":8080"
	DefaultVersion = "dev"

	EnvAddr    = "MM_CHAT_ADDR"
	EnvVersion = "MM_CHAT_VERSION"
)

// Config contains the process-level settings required to start the API.
type Config struct {
	Addr    string
	Version string
}

// Load reads configuration from the process environment.
func Load() Config {
	return LoadFromEnv(os.LookupEnv)
}

// LoadFromEnv reads configuration from the supplied lookup function. Empty or
// whitespace-only values fall back to defaults so a partially configured
// environment still starts with safe development settings.
func LoadFromEnv(lookup func(string) (string, bool)) Config {
	return Config{
		Addr:    envOrDefault(lookup, EnvAddr, DefaultAddr),
		Version: envOrDefault(lookup, EnvVersion, DefaultVersion),
	}
}

func envOrDefault(lookup func(string) (string, bool), key string, fallback string) string {
	value, ok := lookup(key)
	if !ok {
		return fallback
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}
