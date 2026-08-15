package main

import "testing"

func TestParseConfigRequiresEveryBoundedPathAndNoPositionals(t *testing.T) {
	arguments := []string{
		"--podman", "/safe/bin/podman",
		"--workload", "/safe/bin/neo-skill-local-test-workload",
		"--seccomp", "/safe/config/seccomp-agent-v1.json",
		"--state-root", "/safe/smoke-state",
	}
	config, err := parseConfig(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if config.PodmanPath != "/safe/bin/podman" || config.StateRoot != "/safe/smoke-state" {
		t.Fatalf("unexpected parsed config: %#v", config)
	}
	for name, candidate := range map[string][]string{
		"missing":    arguments[:len(arguments)-2],
		"positional": append(append([]string(nil), arguments...), "unexpected"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(candidate); err == nil {
				t.Fatal("invalid local-test arguments were accepted")
			}
		})
	}
}
