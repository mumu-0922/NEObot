package main

import (
	"strings"
	"testing"
)

func TestLoadMigrationConfigReadsOnlyMigrationDatabaseURL(t *testing.T) {
	const runtimeDSN = "postgres://runtime:runtime-secret@runtime.invalid/chat"
	lookups := make([]string, 0, 1)
	cfg, err := loadMigrationConfig(func(name string) (string, bool) {
		lookups = append(lookups, name)
		values := map[string]string{
			"DATABASE_URL":          runtimeDSN,
			migrationDatabaseURLEnv: " postgres://migrate:migrate-secret@db/chat ",
		}
		value, ok := values[name]
		return value, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://migrate:migrate-secret@db/chat" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if len(lookups) != 1 || lookups[0] != migrationDatabaseURLEnv {
		t.Fatalf("environment lookups = %q", lookups)
	}
}

func TestRunFailsClosedWithoutMigrationDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://runtime:runtime-secret@runtime.invalid/chat")
	t.Setenv(migrationDatabaseURLEnv, " \n ")

	err := run([]string{"up"})
	if err == nil || err.Error() != "MIGRATION_DATABASE_URL is required" {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunDoesNotLeakMigrationDatabaseURL(t *testing.T) {
	const credentialSentinel = "migration-secret-do-not-log"
	dsn := "postgres://migration:" + credentialSentinel + "@%zz/chat"
	t.Setenv(migrationDatabaseURLEnv, dsn)

	err := run([]string{"up"})
	if err == nil {
		t.Fatal("run() error = nil")
	}
	if strings.Contains(err.Error(), dsn) ||
		strings.Contains(err.Error(), credentialSentinel) {
		t.Fatalf("run() leaked migration DSN: %v", err)
	}
}

func TestLoadMigrationConfigRejectsMissingValue(t *testing.T) {
	_, err := loadMigrationConfig(func(name string) (string, bool) {
		if name != migrationDatabaseURLEnv {
			t.Fatalf("unexpected environment lookup %q", name)
		}
		return "", false
	})
	if err == nil || err.Error() != "MIGRATION_DATABASE_URL is required" {
		t.Fatalf("loadMigrationConfig() error = %v", err)
	}
}

func TestParseCommandAcceptsGovernanceMappingOnlyForUp(t *testing.T) {
	options, err := parseCommand([]string{
		"up",
		"--phase15-governance-map=/run/secrets/phase15.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.command != "up" || options.governanceMapPath != "/run/secrets/phase15.json" {
		t.Fatalf("options = %#v", options)
	}

	if _, err := parseCommand([]string{
		"down",
		"--phase15-governance-map=/run/secrets/phase15.json",
	}); err == nil {
		t.Fatal("down accepted the up-only governance mapping flag")
	}
}

func TestParseCommandDoesNotEchoFlagValueInUsage(t *testing.T) {
	secretPath := "/run/secrets/release-evidence-do-not-log.json"
	_, err := parseCommand([]string{"baseline", "--phase15-governance-map=" + secretPath})
	if err == nil {
		t.Fatal("parseCommand() error = nil")
	}
	if strings.Contains(err.Error(), secretPath) {
		t.Fatalf("error leaked mapping path: %v", err)
	}
}

func TestParseCommandDownAll(t *testing.T) {
	options, err := parseCommand([]string{"down", "--all"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.downAll {
		t.Fatal("down --all did not set downAll")
	}
}
